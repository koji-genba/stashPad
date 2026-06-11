package csvimport

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// TestImportNoRJNumberColumn は rj_number カラムがない CSV でエラーになることをテスト。
func TestImportNoRJNumberColumn(t *testing.T) {
	db := openTestDB(t)

	// rj_number カラムなし
	csvData := "title,circle,genres\nテスト作品,サークル,ASMR\n"

	_, err := Import(db, strings.NewReader(csvData))
	if err == nil {
		t.Error("rj_number カラムなしなのにエラーにならなかった")
	}
}

// TestImportEmptyRJNumber は rj_number が空の行がエラーとして収集されることをテスト。
func TestImportEmptyRJNumber(t *testing.T) {
	db := openTestDB(t)

	// rj_number が空の行を含む CSV(1行は正常)
	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ111222,正常行,,,,,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
,空RJ行,,,,,ASMR,ボイス・ASMR,MP3,1GB,,全年齢,,,,,
`

	res, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}

	// 空 rj_number 行はエラーに収集される
	if len(res.Errors) == 0 {
		t.Error("空 rj_number 行がエラーとして収集されていない")
	}

	// 正常行は作成される
	if res.Created != 1 {
		t.Errorf("Created = %d, want 1", res.Created)
	}
}

// TestImportEmptyCSV はヘッダのみの CSV(データ行なし)がエラーなく処理されることをテスト。
func TestImportEmptyCSV(t *testing.T) {
	db := openTestDB(t)

	// ヘッダのみ
	csvData := "rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music\n"

	res, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}
	if res.Created != 0 {
		t.Errorf("Created = %d, want 0", res.Created)
	}
	if len(res.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", res.Errors)
	}
}

// TestImportInvalidCSV は CSV 形式が不正な場合にエラーになることをテスト。
func TestImportInvalidCSV(t *testing.T) {
	db := openTestDB(t)

	// 壊れた CSV(引用符が閉じていない; LazyQuotes=true なので通過する場合も)
	// 完全に空のリーダーだとヘッダ読み込みで EOF エラーになる
	res, err := Import(db, strings.NewReader(""))
	if err == nil {
		t.Error("空 CSV でエラーにならなかった")
	}
	_ = res
}

// TestBOMReaderShortData は 3 バイト未満の入力が正しく処理されることをテスト。
func TestBOMReaderShortData(t *testing.T) {
	// 1 バイトだけの入力
	r := newBOMReader(strings.NewReader("x"))
	buf := make([]byte, 10)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if n != 1 || buf[0] != 'x' {
		t.Errorf("Read() = %d バイト %q, want 1バイト 'x'", n, buf[:n])
	}
}

// TestBOMReaderExactlyBOM は 3 バイトが BOM のみの場合、データが空になることをテスト。
func TestBOMReaderExactlyBOM(t *testing.T) {
	bom := "\xef\xbb\xbf"
	r := newBOMReader(strings.NewReader(bom))
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll エラー: %v", err)
	}
	// BOM 3 バイトが除去されて残りは 0 バイトのはず
	if len(data) != 0 {
		t.Errorf("BOM のみの入力で %d バイト読めた: %q, want 0 バイト", len(data), data)
	}
}

// TestBOMReaderNonBOM は BOM でない 3 バイトのデータが保持されることをテスト。
func TestBOMReaderNonBOM(t *testing.T) {
	// BOM でないバイト列
	r := newBOMReader(strings.NewReader("abc"))
	buf := make([]byte, 10)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "abc" {
		t.Errorf("Read() = %q, want abc", string(buf[:n]))
	}
}

// TestBOMReaderSubsequentRead は BOM 除去後に続くデータが正しく読めることをテスト。
func TestBOMReaderSubsequentRead(t *testing.T) {
	bom := "\xef\xbb\xbf"
	data := bom + "hello"
	r := newBOMReader(strings.NewReader(data))

	// 最初の Read: buf が十分大きい場合
	buf := make([]byte, 1) // 1 バイトずつ読む
	var result []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read エラー: %v", err)
		}
	}
	if string(result) != "hello" {
		t.Errorf("BOM 除去後のデータ = %q, want hello", string(result))
	}
}

// TestBOMReaderNonBOMSubsequentRead は BOM なしデータの 2 回目以降の Read が正しく動くことをテスト。
func TestBOMReaderNonBOMSubsequentRead(t *testing.T) {
	// "xyz" は BOM でない。buf が小さいため 2 回目の Read が b.r.Read(p) パスに入る
	r := newBOMReader(strings.NewReader("xyzABCD"))

	buf := make([]byte, 2)
	var result []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read エラー: %v", err)
		}
	}
	if string(result) != "xyzABCD" {
		t.Errorf("data = %q, want xyzABCD", string(result))
	}
}

// errReader は Read を呼ぶと常にエラーを返す io.Reader。
type errReader struct {
	err error
}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, e.err
}

// TestBOMReaderReadError は内部 reader がエラーを返した場合にエラーが伝播することをテスト。
func TestBOMReaderReadError(t *testing.T) {
	want := errors.New("read error")
	r := newBOMReader(&errReader{err: want})
	buf := make([]byte, 10)
	_, err := r.Read(buf)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

// TestImportTitleFallback は title カラムが空の場合に rj_number がタイトルに使われることをテスト。
func TestImportTitleFallback(t *testing.T) {
	db := openTestDB(t)

	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ999001,,,,,,ボイス,ボイス,MP3,1GB,,全年齢,,,,,
`

	res, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("Import 失敗: %v", err)
	}
	if res.Created != 1 {
		t.Errorf("Created = %d, want 1", res.Created)
	}

	// title が rj_number と同じ値になっているか
	var title string
	if err := db.QueryRow("SELECT title FROM works WHERE rj_number='RJ999001'").Scan(&title); err != nil {
		t.Fatalf("SELECT 失敗: %v", err)
	}
	if title != "RJ999001" {
		t.Errorf("title = %q, want RJ999001", title)
	}
}

// TestNullIfEmpty は nullIfEmpty のユニットテスト。
func TestNullIfEmpty(t *testing.T) {
	if got := nullIfEmpty(""); got.Valid {
		t.Error("空文字列が Valid=true で返った")
	}
	if got := nullIfEmpty("hello"); !got.Valid || got.String != "hello" {
		t.Errorf("nullIfEmpty(hello) = %v", got)
	}
}

// TestBuildColumnIndex は buildColumnIndex のユニットテスト。
func TestBuildColumnIndex(t *testing.T) {
	header := []string{"rj_number", " title ", "genres"}
	idx := buildColumnIndex(header)

	if idx["rj_number"] != 0 {
		t.Errorf("rj_number idx = %d, want 0", idx["rj_number"])
	}
	if idx["title"] != 1 {
		t.Errorf("title idx = %d, want 1", idx["title"])
	}
	if idx["genres"] != 2 {
		t.Errorf("genres idx = %d, want 2", idx["genres"])
	}
}

// TestImportMultipleTagsReimport はタグが多い作品を 2 回インポートしても
// タグ件数が正しいことをテスト(二重リンクが起きないことの確認)。
func TestImportMultipleTagsReimport(t *testing.T) {
	db := openTestDB(t)

	csvData := `rj_number,title,series_name,circle,purchase_date,genres,detail_genres,work_type,file_format,file_size,supported_os,age_rating,event,scenario,illustration,voice_actor,music
RJ998001,多タグ作品,,,,"R-15, ボイス・ASMR","ASMR 癒し 耳かき",ボイス・ASMR,MP3,1GB,,全年齢,,,葉月かなめ,耳恋なか/箱河ノア,
`
	// 1 回目: genres×2 + detail_genre×3 + illustration×1 + voice_actor×2 = 8 タグ
	res1, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("1回目 Import 失敗: %v", err)
	}
	if res1.Created != 1 {
		t.Fatalf("1回目 Created = %d, want 1", res1.Created)
	}

	var workID int64
	db.QueryRow("SELECT id FROM works WHERE rj_number='RJ998001'").Scan(&workID)

	var count1 int
	db.QueryRow("SELECT COUNT(*) FROM work_tags WHERE work_id=?", workID).Scan(&count1)

	// 2 回目: 同じデータ、タグ件数が変わっていないことを確認
	res2, err := Import(db, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("2回目 Import 失敗: %v", err)
	}
	if res2.Updated != 1 {
		t.Fatalf("2回目 Updated = %d, want 1", res2.Updated)
	}

	var count2 int
	db.QueryRow("SELECT COUNT(*) FROM work_tags WHERE work_id=?", workID).Scan(&count2)

	if count1 != count2 {
		t.Errorf("再インポート後タグ件数 %d → %d(二重リンクの疑い)", count1, count2)
	}
}
