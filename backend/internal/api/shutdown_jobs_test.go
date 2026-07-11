package api

import (
	"context"
	"testing"
	"time"
)

// ---- バックグラウンドジョブのライフサイクル管理 (issue #83) -------------------
//
// startJob/CancelJobs/WaitJobs は、起動時スキャンやサムネイル一括再生成といった
// バックグラウンドジョブをシャットダウン時に安全に畳むための土台。

// TestStartJobAndWaitJobs は、startJob で登録したジョブが終了するまで
// WaitJobs が待ち、CancelJobs で中断を通知すればジョブが終了して
// WaitJobs が nil を返すようになることをテストする。
func TestStartJobAndWaitJobs(t *testing.T) {
	srv, _ := testServerAndHandler(t)

	started := make(chan struct{})
	done := make(chan struct{})
	srv.startJob(func() {
		close(started)
		<-srv.jobCtx.Done()
		close(done)
	})
	<-started

	// ジョブがまだ jobCtx.Done() を待っている間は、短い期限の WaitJobs は
	// タイムアウトエラーを返すべき。
	shortCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := srv.WaitJobs(shortCtx); err == nil {
		t.Fatal("ジョブが終了していないのに WaitJobs が nil を返した")
	}

	srv.CancelJobs()

	longCtx, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if err := srv.WaitJobs(longCtx); err != nil {
		t.Fatalf("CancelJobs 後の WaitJobs がエラーを返した: %v", err)
	}

	select {
	case <-done:
	default:
		t.Fatal("WaitJobs が nil を返した以上、ジョブは終了しているはず")
	}
}

// TestWaitJobs_NoJobs は、ジョブを1つも起動していなければ WaitJobs が
// 即座に nil を返すことをテストする。
func TestWaitJobs_NoJobs(t *testing.T) {
	srv, _ := testServerAndHandler(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := srv.WaitJobs(ctx); err != nil {
		t.Fatalf("ジョブ未起動時の WaitJobs = %v, want nil", err)
	}
}
