package appservice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"mypack.dev/mypack/internal/domain"
)

// fakeLogin is the system's login-item record. hold keeps a requested
// change from taking effect, as macOS does while it waits for approval.
type fakeLogin struct {
	mu      sync.Mutex
	on      bool
	hold    bool
	failSet error
	sets    []bool
}

func (f *fakeLogin) Enabled() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.on }
func (f *fakeLogin) SetEnabled(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, on)
	if f.failSet != nil {
		return f.failSet
	}
	if !f.hold {
		f.on = on
	}
	return nil
}

func withLogin(l LoginItem) func(*Options) { return func(o *Options) { o.LoginItem = l } }

func errCode(err error) string {
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}

func TestOpenAtLoginReadsTheSystem(t *testing.T) {
	ctx := context.Background()
	l := &fakeLogin{}
	e := newEnv(t, withLogin(l))

	if on, err := e.svc.OpenAtLogin(ctx); err != nil || on {
		t.Fatalf("OpenAtLogin = %v, %v; want false", on, err)
	}
	// Turned on outside MyPack (System Settings, Task Manager): it shows.
	l.mu.Lock()
	l.on = true
	l.mu.Unlock()
	if on, _ := e.svc.OpenAtLogin(ctx); !on {
		t.Fatal("OpenAtLogin did not follow the system")
	}
	// Nothing about it is kept in Settings, so it cannot drift.
	if _, err := e.svc.store.GetSetting(ctx, "openAtLogin", new(bool)); err == nil {
		// GetSetting's not-found behaviour is store-specific; only a stored
		// value would be a problem.
		var v bool
		if ok, _ := e.svc.store.GetSetting(ctx, "openAtLogin", &v); ok {
			t.Fatal("open at login was stored as a setting")
		}
	}
}

func TestSetOpenAtLogin(t *testing.T) {
	ctx := context.Background()
	l := &fakeLogin{}
	e := newEnv(t, withLogin(l))
	var events []EventKind
	var mu sync.Mutex
	defer e.svc.Subscribe(func(ev Event) { mu.Lock(); events = append(events, ev.Kind); mu.Unlock() })()

	if err := e.svc.SetOpenAtLogin(ctx, true, "gui"); err != nil {
		t.Fatal(err)
	}
	if on, _ := e.svc.OpenAtLogin(ctx); !on {
		t.Fatal("not on after SetOpenAtLogin(true)")
	}
	if err := e.svc.SetOpenAtLogin(ctx, false, "gui"); err != nil {
		t.Fatal(err)
	}
	if on, _ := e.svc.OpenAtLogin(ctx); on {
		t.Fatal("still on after SetOpenAtLogin(false)")
	}
	if len(l.sets) != 2 || !l.sets[0] || l.sets[1] {
		t.Fatalf("system asked %v; want [true false]", l.sets)
	}
	acts, _ := e.svc.Activity(ctx, 10)
	n := 0
	for _, a := range acts {
		if a.Action == "settings.openAtLogin" && a.Result == "ok" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("%d ok settings.openAtLogin activities; want 2", n)
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		c := 0
		for _, k := range events {
			if k == EventSettingsChanged {
				c++
			}
		}
		return c == 2
	})
}

func TestSetOpenAtLoginFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("system refuses", func(t *testing.T) {
		e := newEnv(t, withLogin(&fakeLogin{failSet: errors.New("needs an app bundle")}))
		err := e.svc.SetOpenAtLogin(ctx, true, "gui")
		if errCode(err) != CodeOpenAtLogin {
			t.Fatalf("err = %v; want %s", err, CodeOpenAtLogin)
		}
		var de *domain.Error
		errors.As(err, &de)
		if de.Cause != "needs an app bundle" || de.Remediation == "" {
			t.Fatalf("error lost the cause or remediation: %+v", de)
		}
	})

	t.Run("held for approval", func(t *testing.T) {
		l := &fakeLogin{hold: true}
		e := newEnv(t, withLogin(l))
		if err := e.svc.SetOpenAtLogin(ctx, true, "gui"); errCode(err) != CodeOpenAtLogin {
			t.Fatalf("err = %v; want %s when the system did not take it", err, CodeOpenAtLogin)
		}
		if on, _ := e.svc.OpenAtLogin(ctx); on {
			t.Fatal("reported on while the system held it")
		}
	})

	t.Run("no login item (CLI, MCP)", func(t *testing.T) {
		e := newEnv(t)
		if _, err := e.svc.OpenAtLogin(ctx); errCode(err) != CodeUnsupported {
			t.Fatalf("OpenAtLogin err = %v; want %s", err, CodeUnsupported)
		}
		if err := e.svc.SetOpenAtLogin(ctx, true, "cli"); errCode(err) != CodeUnsupported {
			t.Fatalf("SetOpenAtLogin err = %v; want %s", err, CodeUnsupported)
		}
	})
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
