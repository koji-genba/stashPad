// ユーザー付与メタデータ(カスタムタグ・お気に入り・非表示・手動編集)のエクスポート/
// インポート。DB を作り直して再デプロイする際のバックアップ・復元用(issue #78)。
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// metadataSchemaVersion は GET /api/export / POST /api/import/metadata が扱う
// JSON スキーマのバージョン。将来フォーマットを変える場合はこの値を上げ、
// import 側は非対応バージョンを 400 で弾く。
const metadataSchemaVersion = 1

// metadataWorkItem は GET /api/export の works 要素、および
// POST /api/import/metadata の works 要素(同じ形)。
// null 許容フィールドは *string にして明示的に null を返す(既存 API の流儀。issue #57)。
type metadataWorkItem struct {
	RJNumber       *string  `json:"rj_number"`
	RootPath       *string  `json:"root_path"`
	Title          string   `json:"title"`
	Circle         *string  `json:"circle"`
	ManuallyEdited bool     `json:"manually_edited"`
	Hidden         bool     `json:"hidden"`
	FavoritedAt    *string  `json:"favorited_at"`
	CustomTags     []string `json:"custom_tags"`
}

// exportMetadataResponse は GET /api/export のレスポンス全体。
type exportMetadataResponseBody struct {
	Version    int                `json:"version"`
	ExportedAt string             `json:"exported_at"`
	Works      []metadataWorkItem `json:"works"`
}

// handleExportMetadata は GET /api/export を処理する。
// カスタムタグ・お気に入り・非表示・手動編集のいずれか 1 つ以上を持つ作品だけを
// JSON でダウンロードさせる(何も無い作品は対象外)。
func (s *Server) handleExportMetadata(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT w.id, w.rj_number, w.root_path, w.title, w.circle,
		       w.manually_edited, w.hidden, w.favorited_at
		FROM works w
		WHERE w.favorited_at IS NOT NULL
		   OR w.hidden = 1
		   OR w.manually_edited = 1
		   OR EXISTS (
		        SELECT 1 FROM work_tags wt
		        JOIN tags t ON t.id = wt.tag_id
		        WHERE wt.work_id = w.id AND t.category = 'custom'
		      )
		ORDER BY w.id`)
	if err != nil {
		respondInternalError(w, "エクスポート対象取得失敗", err)
		return
	}

	type workRow struct {
		id             int64
		rjNumber       sql.NullString
		rootPath       sql.NullString
		title          string
		circle         sql.NullString
		manuallyEdited int
		hidden         int
		favoritedAt    sql.NullString
	}
	var collected []workRow
	for rows.Next() {
		var wr workRow
		if err := rows.Scan(&wr.id, &wr.rjNumber, &wr.rootPath, &wr.title, &wr.circle,
			&wr.manuallyEdited, &wr.hidden, &wr.favoritedAt); err != nil {
			rows.Close()
			respondInternalError(w, "行読み込み失敗", err)
			return
		}
		collected = append(collected, wr)
	}
	closeErr := rows.Close()
	if err := rows.Err(); err != nil {
		respondInternalError(w, "行読み込み失敗", err)
		return
	}
	if closeErr != nil {
		respondInternalError(w, "行読み込み失敗", closeErr)
		return
	}

	works := make([]metadataWorkItem, 0, len(collected))
	for _, wr := range collected {
		tags, err := s.customTagsForWork(wr.id)
		if err != nil {
			respondInternalError(w, "カスタムタグ取得失敗", err)
			return
		}
		works = append(works, metadataWorkItem{
			RJNumber:       nullableString(wr.rjNumber),
			RootPath:       nullableString(wr.rootPath),
			Title:          wr.title,
			Circle:         nullableString(wr.circle),
			ManuallyEdited: wr.manuallyEdited != 0,
			Hidden:         wr.hidden != 0,
			FavoritedAt:    nullableString(wr.favoritedAt),
			CustomTags:     tags,
		})
	}

	resp := exportMetadataResponseBody{
		Version:    metadataSchemaVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Works:      works,
	}

	// ブラウザ直リンクでダウンロード可能にする。日付部分のみのファイル名なので
	// RFC5987 エンコード(handleWorkFile の他ファイル用)は不要。
	filename := fmt.Sprintf("stashpad-metadata-%s.json", time.Now().Format("20060102"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	respondJSON(w, http.StatusOK, resp)
}

// customTagsForWork は work_id に紐づく category='custom' のタグ名一覧を名前昇順で返す。
func (s *Server) customTagsForWork(workID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT t.name FROM tags t
		JOIN work_tags wt ON wt.tag_id = t.id
		WHERE wt.work_id = ? AND t.category = 'custom'
		ORDER BY t.name`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// ---- インポート ---------------------------------------------------------------

// maxMetadataUploadBytes は POST /api/import/metadata が受け付けるリクエストボディの上限。
// CSV インポート(handlers_csv.go)と同じ考え方で上限を設ける。
const maxMetadataUploadBytes = 32 << 20 // 32MB

// importMetadataRequest は POST /api/import/metadata のリクエストボディ
// (GET /api/export のレスポンスをそのまま送り返せる形)。
type importMetadataRequest struct {
	Version int                `json:"version"`
	Works   []metadataWorkItem `json:"works"`
}

// importMetadataResult は POST /api/import/metadata のレスポンス。
type importMetadataResult struct {
	Matched   int      `json:"matched"`
	Skipped   int      `json:"skipped"`
	TagsAdded int      `json:"tags_added"`
	Errors    []string `json:"errors"`
}

// handleImportMetadata は POST /api/import/metadata を処理する。
// エクスポートされた JSON からユーザー付与メタデータを復元する。
//
// 復元は加算のみで既存データを消さない(design.md 決定事項ログ参照。issue #78):
//   - custom_tags は upsert して work_tags に追加するのみ(既存タグは維持)
//   - favorited_at はエクスポート値が非 null のときだけ SET(上書き可)
//   - hidden は true のときだけ 1 に SET(false→クリアはしない)
//   - manually_edited は true のときだけ title/circle を復元して 1 を SET
//
// 全体を単一トランザクションで実行するが、行単位の処理失敗は errors に積んで継続する
// (matched のカウントには影響しない。CSV インポータと同じ考え方)。
func (s *Server) handleImportMetadata(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMetadataUploadBytes)

	var req importMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondError(w, http.StatusRequestEntityTooLarge, "アップロードサイズが大きすぎます: "+err.Error())
			return
		}
		respondError(w, http.StatusBadRequest, "JSON パース失敗: "+err.Error())
		return
	}

	if req.Version != metadataSchemaVersion {
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("対応していない version です: %d(対応: %d)", req.Version, metadataSchemaVersion))
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		respondInternalError(w, "トランザクション開始失敗", err)
		return
	}

	result := importMetadataResult{Errors: []string{}}
	var txErr error
	for _, item := range req.Works {
		identifier := metadataIdentifier(item)

		workID, found, findErr := findWorkForMetadata(tx, item)
		if findErr != nil {
			txErr = fmt.Errorf("照合失敗 %s: %w", identifier, findErr)
			break
		}
		if !found {
			result.Skipped++
			continue
		}
		result.Matched++

		added, applyErr := applyMetadataItem(tx, workID, item)
		result.TagsAdded += added
		if applyErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", identifier, applyErr))
		}
	}

	if txErr != nil {
		tx.Rollback()
		respondInternalError(w, "メタデータインポート失敗", txErr)
		return
	}
	if err := tx.Commit(); err != nil {
		respondInternalError(w, "コミット失敗", err)
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// metadataIdentifier はエラーメッセージ用の識別子("rj_number/RJxxx: ..." 形式)を返す。
// rj_number が無ければ root_path、それも無ければ title にフォールバックする。
func metadataIdentifier(item metadataWorkItem) string {
	if item.RJNumber != nil && *item.RJNumber != "" {
		return *item.RJNumber
	}
	if item.RootPath != nil && *item.RootPath != "" {
		return *item.RootPath
	}
	return item.Title
}

// findWorkForMetadata は rj_number があれば rj_number で、無ければ root_path で
// works を検索する。見つからなければ found=false(スキップ対象)を返す。
func findWorkForMetadata(tx *sql.Tx, item metadataWorkItem) (workID int64, found bool, err error) {
	var scanErr error
	switch {
	case item.RJNumber != nil && *item.RJNumber != "":
		scanErr = tx.QueryRow("SELECT id FROM works WHERE rj_number=?", *item.RJNumber).Scan(&workID)
	case item.RootPath != nil && *item.RootPath != "":
		scanErr = tx.QueryRow("SELECT id FROM works WHERE root_path=?", *item.RootPath).Scan(&workID)
	default:
		// 照合キー(rj_number も root_path も)が無い行は照合先なしとして扱う。
		return 0, false, nil
	}
	if scanErr == sql.ErrNoRows {
		return 0, false, nil
	}
	if scanErr != nil {
		return 0, false, scanErr
	}
	return workID, true, nil
}

// applyMetadataItem は 1 件分のメタデータを加算のみのセマンティクスで works に反映する。
// 戻り値は新たに追加された custom タグのリンク数(tags_added への加算分)。
func applyMetadataItem(tx *sql.Tx, workID int64, item metadataWorkItem) (tagsAdded int, err error) {
	// custom タグ: upsert してから work_tags へ INSERT OR IGNORE(既存タグには触れない)
	seen := make(map[string]bool, len(item.CustomTags))
	for _, name := range item.CustomTags {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		if _, err = tx.Exec(
			"INSERT INTO tags (name, category) VALUES (?, 'custom') ON CONFLICT(name, category) DO NOTHING",
			name,
		); err != nil {
			return tagsAdded, fmt.Errorf("タグ作成失敗 %q: %w", name, err)
		}
		var tagID int64
		if err = tx.QueryRow(
			"SELECT id FROM tags WHERE name=? AND category='custom'", name,
		).Scan(&tagID); err != nil {
			return tagsAdded, fmt.Errorf("タグ ID 取得失敗 %q: %w", name, err)
		}
		var res sql.Result
		res, err = tx.Exec(
			"INSERT INTO work_tags (work_id, tag_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
			workID, tagID,
		)
		if err != nil {
			return tagsAdded, fmt.Errorf("タグリンク失敗 %q: %w", name, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			tagsAdded++
		}
	}

	// favorited_at: エクスポート値が非 null のときだけ SET(既存が非 NULL でも上書き可)
	if item.FavoritedAt != nil {
		if _, err = tx.Exec(
			"UPDATE works SET favorited_at=?, updated_at=datetime('now') WHERE id=?",
			*item.FavoritedAt, workID,
		); err != nil {
			return tagsAdded, fmt.Errorf("favorited_at 更新失敗: %w", err)
		}
	}

	// hidden: true のときだけ 1 に SET(false→クリアは一切しない)
	if item.Hidden {
		if _, err = tx.Exec(
			"UPDATE works SET hidden=1, updated_at=datetime('now') WHERE id=?", workID,
		); err != nil {
			return tagsAdded, fmt.Errorf("hidden 更新失敗: %w", err)
		}
	}

	// manually_edited: true のときだけ title/circle を復元して manually_edited=1 を SET
	if item.ManuallyEdited {
		if _, err = tx.Exec(
			"UPDATE works SET title=?, circle=?, manually_edited=1, updated_at=datetime('now') WHERE id=?",
			item.Title, item.Circle, workID,
		); err != nil {
			return tagsAdded, fmt.Errorf("title/circle 復元失敗: %w", err)
		}
	}

	return tagsAdded, nil
}
