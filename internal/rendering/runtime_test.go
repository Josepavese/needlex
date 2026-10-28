package rendering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
)

func TestRuntimeIsolatesStorageAndDisposesContexts(t *testing.T) {
	browser := runnableBrowserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/sw.js" {
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `self.addEventListener('install',()=>self.skipWaiting());self.addEventListener('activate',e=>e.waitUntil(clients.claim()));`)
			return
		}
		fmt.Fprint(w, `<!doctype html><p id="state">pending</p><script>
(async()=>{
 const priorCookie=document.cookie, priorLocal=localStorage.getItem('private'), priorSession=sessionStorage.getItem('private');
 const cacheKeys=await caches.keys(), registrations=await navigator.serviceWorker.getRegistrations();
 const db=await new Promise((resolve,reject)=>{let q=indexedDB.open('private',1);q.onupgradeneeded=()=>q.result.createObjectStore('values');q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error)});
 const priorIDB=await new Promise(resolve=>{let q=db.transaction('values').objectStore('values').get('secret');q.onsuccess=()=>resolve(q.result)});
 const fresh=!priorCookie&&!priorLocal&&!priorSession&&!priorIDB&&!cacheKeys.length&&!registrations.length;
 localStorage.setItem('private','secret');sessionStorage.setItem('private','secret');document.cookie='private=secret;path=/';
 await new Promise(resolve=>{let tx=db.transaction('values','readwrite');tx.objectStore('values').put('secret','secret');tx.oncomplete=resolve});db.close();
 await (await caches.open('private')).put('/secret',new Response('private payload'));
 await navigator.serviceWorker.register('/sw.js');await navigator.serviceWorker.ready;
 document.querySelector('#state').textContent=(fresh?'fresh':'LEAK')+'|'+navigator.userAgent;
})().catch(e=>document.querySelector('#state').textContent='ERROR:'+e.message);
</script>`)
	}))
	defer server.Close()
	cfg := config.DefaultRenderConfig()
	cfg.BrowserPath = browser
	cfg.NetworkIdleMS = 500
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close() //nolint:errcheck
	for i, ua := range []string{"NeedleFixture/first", "NeedleFixture/second"} {
		page, err := runtime.Render(context.Background(), Request{URL: server.URL, UserAgent: ua, Timeout: 10 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if page.Degraded || !strings.Contains(page.HTML, `id="state">fresh|`+ua+`</p>`) {
			t.Fatalf("isolation/UA failed: degraded=%t html=%s", page.Degraded, page.HTML)
		}
		if page.BrowserReused != (i > 0) {
			t.Fatalf("read %d reused=%t", i, page.BrowserReused)
		}
		assertNoBrowserContexts(t, runtime.(*managedRenderer).browser, browser)
	}
}

func assertNoBrowserContexts(t *testing.T, host platform.BrowserRuntime, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := host.Acquire(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release(true) //nolint:errcheck
	conn, _, err := websocket.Dial(ctx, lease.Endpoint(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow() //nolint:errcheck
	cdp := cdpClient{conn: conn}
	data, err := cdp.command(ctx, "", "Target.getBrowserContexts", nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Contexts []string `json:"browserContextIds"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Contexts) != 0 {
		t.Fatalf("contexts survived render: %v", result.Contexts)
	}
}

func TestRuntimeCancellationAndCloseCannotLaunchFallback(t *testing.T) {
	browser := runnableBrowserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/hang" {
			fmt.Fprint(w, `<p>pending</p><script>while(true){}</script>`)
			return
		}
		fmt.Fprint(w, `<p>Recovered browser content is available after cancellation.</p>`)
	}))
	defer server.Close()
	cfg := config.DefaultRenderConfig()
	cfg.BrowserPath = browser
	cfg.NetworkIdleMS = 100
	r, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	_, err = r.Render(ctx, Request{URL: server.URL + "/hang"})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline=%v", err)
	}
	page, err := r.Render(context.Background(), Request{URL: server.URL, Timeout: 5 * time.Second})
	if err != nil || page.Degraded || !strings.Contains(page.HTML, "Recovered browser content") {
		t.Fatalf("recovery err=%v page=%+v", err, page)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(context.Background(), Request{URL: server.URL}); !errors.Is(err, platform.ErrBrowserRuntimeClosed) {
		t.Fatalf("closed render=%v", err)
	}
}

func TestRuntimeAppliesConfiguredUseAndIdleBudgets(t *testing.T) {
	browser := runnableBrowserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<p>Bounded browser lifetime content.</p>")
	}))
	defer server.Close()
	cfg := config.DefaultRenderConfig()
	cfg.BrowserPath = browser
	cfg.BrowserMaxUses = 2
	cfg.BrowserIdleTimeoutMS = 150
	cfg.NetworkIdleMS = 50
	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close() //nolint:errcheck
	for i, want := range []bool{false, true, false} {
		page, err := runtime.Render(context.Background(), Request{URL: server.URL})
		if err != nil || page.Degraded {
			t.Fatalf("read %d err=%v degraded=%t", i, err, page.Degraded)
		}
		if page.BrowserReused != want {
			t.Fatalf("read %d reused=%t want=%t", i, page.BrowserReused, want)
		}
	}
	time.Sleep(300 * time.Millisecond)
	page, err := runtime.Render(context.Background(), Request{URL: server.URL})
	if err != nil || page.BrowserReused {
		t.Fatalf("idle budget not applied: reused=%t err=%v", page.BrowserReused, err)
	}
}

func TestOneShotRendererKeepsServiceConcurrencyLimit(t *testing.T) {
	cfg := config.DefaultRenderConfig()
	cfg.MaxConcurrency = 1
	renderer := New(cfg).(oneShotRenderer)
	renderer.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := renderer.Render(ctx, Request{URL: "http://fixture.invalid"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter bypassed service limit: %v", err)
	}
	<-renderer.slots
}
