// Package web はビルド済みフロントエンド(Vite の dist)を go:embed で配信する。
// 実際の成果物は `npm run build` 後に dist/ へコピーされる(.gitkeep のみコミット)。
// 成果物が無い状態でもビルド・テストが通るよう、index.html 不在時は 503 を返す。
package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// extraContentTypes は mime.TypeByExtension / OS の /etc/mime.types に
// 登録されていない拡張子の Content-Type を補う。
// distroless の本番イメージには /etc/mime.types が無く、Go の組み込みテーブルにも
// .webmanifest は含まれないため、明示しないと text/plain 等で配信されてしまう
// (PWA の manifest は application/manifest+json での配信が期待される)。
var extraContentTypes = map[string]string{
	".webmanifest": "application/manifest+json",
}

// Handler は SPA 配信ハンドラを返す。
// 存在するファイルはそのまま配信し、クライアントルート(/works/123 等)には
// index.html を返すフォールバックを行う。/api は呼び出し側で先にマッチさせること。
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed 構成エラーはビルド時に気付くべき問題
	}
	return newHandler(sub)
}

// newHandler は Handler() の本体。テストから実際の embed FS の代わりに
// fstest.MapFS 等を注入できるよう切り出している。
func newHandler(sub fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" && p != "index.html" && exists(sub, p) {
			if ct, ok := extraContentTypes[path.Ext(p)]; ok {
				w.Header().Set("Content-Type", ct)
			}
			// ハッシュ付きアセットは長期キャッシュ可
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// index.html(直接アクセス or SPA フォールバック)
		f, err := sub.Open("index.html")
		if err != nil {
			http.Error(w, "frontend not built", http.StatusServiceUnavailable)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		io.Copy(w, f)
	})
}

// exists は通常ファイルとして開けるかを判定する(ディレクトリは除外)。
func exists(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	return err == nil && !st.IsDir()
}
