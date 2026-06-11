# 再生キュー操作(追い足し・並び替え・削除)の設計ノート

将来この機能を実装する人への申し送り。2026-06 のフルスクリーンプレイヤー実装(PR 時点)で
コードベースを検査し、要件のすり合わせと設計検討まで行ったが、変更規模が大きいため実装は見送った。

## 確定済みの要件(オーナー確認済み)

- キューへの**追い足し**と**並び替え**を実装する
- **削除**も含める(追い足しの取り消し手段として対になる操作のため)
- **他作品のトラックの混在を許す**(再生中の作品 A のキューに作品 B のファイルを追加できる)
- 並び替え UI は**行ごとの ▲▼ ボタン**(ドラッグ&ドロップは採用しない。依存追加なし・
  タッチスクロールと競合しない・テスト容易のため)
- 「作品まるごと再生」「キューの永続化」は**含まない**(永続化は design.md Phase 3 の
  「再生位置の記憶」と合わせて検討)

## 現状の構造と、効いてくる制約

- キュー生成は `playerStore.startFromEntries` ただ一つ。呼び出し元も FileBrowser の
  ファイルタップただ一つ。「タップしたファイルと同じディレクトリの audio を自然順で
  スナップショット」(design.md §8.4)
- `QueueTrack` は `{path, name}` のみで、作品情報はキュー全体で 1 つの
  `ctx: {workId, workTitle, dir}` に持っている。**他作品混在の最大の障害はこれ**
- `ctx.dir` はキュー構築時の joinPath にしか使っておらず、構築後は誰も読まない
- 状態遷移(`playIndex` / `next` / `prev` / `handleEnded`)は store に隔離され、
  テストが厚い(playerStore.test.ts)。store への操作追加は安全にやれる

## 推奨設計

### 1. ctx 廃止 → トラック単位の作品情報

```ts
export interface QueueTrack {
  workId: number;
  workTitle: string;
  path: string;   // 作品ルートからの相対パス
  name: string;
}
// PlayerContext / ctx は削除し、現在トラックから導出する
```

影響箇所(全て ctx 参照を現在トラック参照に置き換える):

- `currentSrc` → `fileUrl(t.workId, t.path)`
- `playerThumbUrl(ctx)` → `playerThumbUrl(track)`(Media Session のアートワーク含む)
- `recordPlayFor` → `recordPlay(t.workId, t.path)`
- AudioPlayer の表示ガード `if (!ctx)` → `if (!track)`、Media Session metadata と
  effect deps も track 基準に
- FullscreenPlayer のヘッダタイトル・navigate 先・アートワーク
- App.tsx の `hasPlayer` → `s.queue.length > 0`
- 各テストの initialState / setState(**アサーションの意図は変えないこと**)

### 2. 新アクションのセマンティクス(このままテストケースにできる粒度)

```ts
enqueueTrack: (track: QueueTrack) => void;
moveInQueue: (from: number, to: number) => void;
removeFromQueue: (index: number) => void;
```

**enqueueTrack**
- キューが空: `queue=[track], index=0, isPlaying=true, currentTime=0, duration=0,
  loadNonce++` で即再生開始し、recordPlay を記録(startFromEntries と同じ扱い)
- キューが空でない: 末尾に追加するだけ。index・isPlaying・loadNonce は触らない。履歴記録なし
- 同一トラックの重複追加は許容(弾かない)

**moveInQueue(from, to)**
- 範囲外 or from===to は何もしない。splice で移動
- **再生中トラックの index 追従**: from===index なら index=to。
  from<index かつ to>=index なら index−1。from>index かつ to<=index なら index+1
- 再生は中断しない(loadNonce・currentTime に触らない)

**removeFromQueue(i)**
- i < index: 除去して index−1(再生継続)。i > index: 除去のみ
- i === index で残りが自分のみ: 全クリア
  (`queue=[], index=-1, isPlaying=false, currentTime=0, duration=0, expanded=false`。
  **expanded を畳み忘れると、次回再生開始時にフルスクリーンが勝手に開く**)
- i === index で他にあり: 除去後 `index = Math.min(i, queue.length-1)`(末尾を消したら
  新しい末尾へ)、`currentTime=0, duration=0, loadNonce++` で次をロード。isPlaying は維持。
  handleEnded の自動送りと同様に recordPlay を記録

### 3. UI

- **FileBrowser**: audio 行にのみ「＋」(aria-label「キューに追加」)。現在は行全体が
  1 つの `<button>` なので、**button のネストにならないよう** li 内を
  flex の div(行本体 button + ＋ button の兄弟)に再構成する必要がある
- **FullscreenPlayer のキュー一覧**: 各行に ▲▼✕。▲は先頭・▼は末尾で disabled。
  他作品混在に備えトラック名の下に作品名を dim 表示
- 行の key: 現在は `t.path` だが、重複追加と作品間のパス衝突で破綻する。
  `` `${t.workId}:${t.path}:${i}` `` のような複合キーにする

## その他の罠

- `prev` はキュー 1 件で disabled のため「曲頭に戻る」用途に使えない(既知の挙動)。
  キュー操作とは独立だが、UI をいじるならついでに見直す余地あり
- `playIndex` には `record: false` の逃げ道が既にある。キュー操作で履歴が汚れる場合はこれを使う
- jsdom は `HTMLMediaElement.play/pause/load` 未実装。AudioPlayer.test.tsx のスタブを参照
- テストは t-wada 流 TDD(Red → Green → リファクタ)で。既存テストの慣習
  (日本語 describe/it・resetStore パターン・@/api/client の vi.mock)は
  playerStore.test.ts / FullscreenPlayer.test.tsx に手本がある
