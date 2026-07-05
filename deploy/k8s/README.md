# stashPad — k8s マニフェスト

`deploy/docker-compose.yml`(開発・お試し用)と同レベルの単純構成を k8s 向けに用意したもの(design.md §10)。
SQLite を使う都合上、**レプリカは 1 固定**・更新戦略は **Recreate** にしている(ローリングアップデートで新旧 Pod が同じ PVC/DB を同時に掴むと壊れるため)。

## ファイル構成

| ファイル | 内容 |
|---|---|
| `deployment.yaml` | stashPad 本体(replicas: 1、Recreate、非 root 実行、probe 設定) |
| `service.yaml` | ClusterIP Service(port 80 → コンテナ 8080) |
| `pvc.yaml` | データボリューム(SQLite + サムネイルキャッシュ、`/data`) |
| `ingress.yaml` | 既存 Ingress Controller への組み込み例 |
| `kustomization.yaml` | 上記 4 ファイルをまとめて適用するための一覧(任意) |

## プレースホルダ一覧(適用前に書き換えること)

| ファイル | 項目 | 説明 |
|---|---|---|
| `deployment.yaml` | `image: REPLACE_ME/stashpad:latest` | リポジトリルートの `Dockerfile` でビルドしたイメージを、クラスタから pull 可能なレジストリに push してから指定する |
| `deployment.yaml` | `nfs.server` / `nfs.path`(`media-voice` / `media-comic`) | ファイルサーバの NFS エクスポート先。ライブラリルートを増減する場合は `volumeMounts` / `volumes` / `STASHPAD_LIBRARY_ROOTS` を対応させて編集する。NFS が使えない場合は末尾のコメントにある `hostPath` 例に置き換える |
| `pvc.yaml` | `storageClassName`(コメントアウト) | 未指定ならクラスタのデフォルト StorageClass を使用する。特定の StorageClass を使う場合のみコメントを外して指定する |
| `pvc.yaml` | `resources.requests.storage` | 目安値(20Gi)。サムネイルキャッシュはライブラリ規模に比例して増えるため実環境に合わせて調整する |
| `ingress.yaml` | `ingressClassName: nginx` | クラスタに導入済みの IngressClass 名に合わせる |
| `ingress.yaml` | `host: stashpad.example.internal` | 実際のホスト名に置き換える |

`/data`(PVC)の書き込み権限は、Deployment の Pod レベル `securityContext.fsGroup: 65532` によって自動的に処理される(対応する CSI ドライバであれば、マウント時にボリュームの GID を自動調整する)。compose 版のように `chown 65532:65532 data` を手動で行う必要はない。

## 適用手順

```bash
# kustomize でまとめて適用する場合
kubectl apply -k deploy/k8s/

# または個別に適用する場合
kubectl apply -f deploy/k8s/pvc.yaml -f deploy/k8s/deployment.yaml -f deploy/k8s/service.yaml -f deploy/k8s/ingress.yaml
```

## 運用上の注意

- stashPad は認証なしで動作する(design.md §9)。Ingress を経由する場合も、VPN 等の信頼できるネットワークからのみ到達可能にすること。直接インターネットへ公開しない
- メディアボリュームは必ず `readOnly: true` でマウントする(design.md の「メディアには書き込まない」方針)
- `startupProbe` は用意していない。`STASHPAD_SCAN_ON_START` によるライブラリスキャンは goroutine でバックグラウンド実行され、HTTP サーバは起動直後に Listen を開始するため、スキャン時間が起動判定を妨げることはない(詳細は `deployment.yaml` のコメントを参照)
