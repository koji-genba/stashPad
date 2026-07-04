package main

import "testing"

// TestHealthzURL は STASHPAD_ADDR 形式のリッスンアドレスから
// ヘルスチェック用 URL を正しく組み立てられることをテストする。
func TestHealthzURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "コロン+ポートのみ",
			addr: ":8080",
			want: "http://localhost:8080/api/healthz",
		},
		{
			name: "ホスト+ポート指定",
			addr: "0.0.0.0:9000",
			want: "http://localhost:9000/api/healthz",
		},
		{
			name: "未設定はデフォルトポート8080",
			addr: "",
			want: "http://localhost:8080/api/healthz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := healthzURL(tt.addr)
			if got != tt.want {
				t.Errorf("healthzURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
