package app

import (
	"reflect"
	"testing"
	"time"

	"mypack.dev/mypack/internal/desktop/kit"
	"mypack.dev/mypack/internal/domain"
)

func TestStepPlan(t *testing.T) {
	cases := []struct {
		name                       string
		project, download, install bool
		want                       []stepKind
	}{
		{"url, cached engine", false, false, false, []stepKind{stepMake, stepSign, stepCheck}},
		{"url, first build", false, true, false, []stepKind{stepEngine, stepMake, stepSign, stepCheck}},
		{"url, install & open", false, false, true, []stepKind{stepMake, stepSign, stepCheck, stepInstall}},
		{"project, cached", true, false, false, []stepKind{stepProject, stepMake, stepSign, stepCheck}},
		{"project, all", true, true, true, []stepKind{stepProject, stepEngine, stepMake, stepSign, stepCheck, stepInstall}},
	}
	for _, c := range cases {
		if got := stepPlan(c.project, c.download, c.install); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: plan %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStepAt(t *testing.T) {
	withEngine := stepPlan(false, true, false)
	cached := stepPlan(false, false, false)
	for _, c := range []struct {
		stage domain.Stage
		plan  []stepKind
		want  stepKind
	}{
		{domain.StagePreparing, stepPlan(true, false, false), stepProject},
		{domain.StageValidating, withEngine, stepEngine},
		{domain.StageValidating, cached, stepMake}, // the checks count toward the first step
		{domain.StageDownloading, withEngine, stepEngine},
		{domain.StageCustomizing, cached, stepMake},
		{domain.StageSigning, cached, stepSign},
		{domain.StagePublishing, cached, stepCheck},
		{domain.StageCompleted, cached, stepCheck},
	} {
		if got := stepAt(c.stage, c.plan); got != c.want {
			t.Errorf("stepAt(%s) = %v, want %v", c.stage, got, c.want)
		}
	}
}

func TestStepStates(t *testing.T) {
	P, A, D, F, S := kit.StepPending, kit.StepActive, kit.StepDone, kit.StepFailed, kit.StepSkipped
	plan := stepPlan(true, true, true) // project, engine, make, sign, check, install
	for _, c := range []struct {
		name string
		cur  stepKind
		ph   stepPhase
		want []kit.StepState
	}{
		{"waiting for approval", stepProject, phaseWaiting, []kit.StepState{P, P, P, P, P, P}},
		{"building the project", stepProject, phaseRunning, []kit.StepState{A, P, P, P, P, P}},
		{"downloading", stepEngine, phaseRunning, []kit.StepState{D, A, P, P, P, P}},
		{"signing", stepSign, phaseRunning, []kit.StepState{D, D, D, A, P, P}},
		{"failed signing", stepSign, phaseFailed, []kit.StepState{D, D, D, F, P, P}},
		{"cancelled", stepMake, phaseCancelled, []kit.StepState{D, D, S, P, P, P}},
		{"built, installing", stepCheck, phaseInstalling, []kit.StepState{D, D, D, D, D, A}},
		{"installed", stepCheck, phaseInstalled, []kit.StepState{D, D, D, D, D, D}},
		{"not installed", stepCheck, phaseNotInstalled, []kit.StepState{D, D, D, D, D, S}},
	} {
		if got := stepStates(plan, c.cur, c.ph); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if !stepsDone(stepStates(plan, stepCheck, phaseInstalled)) || stepsDone(stepStates(plan, stepCheck, phaseInstalling)) {
		t.Error("stepsDone: the list gives way only once Install & Open has finished")
	}
}

func TestStepTrackDurations(t *testing.T) {
	plan := stepPlan(false, true, false) // engine, make, sign, check
	tr := newStepTrack(plan)
	t0 := time.Unix(1000, 0)
	at := func(cur stepKind, ph stepPhase, sec float64) {
		tr.observe(stepStates(plan, cur, ph), t0.Add(time.Duration(sec*float64(time.Second))))
	}
	at(stepEngine, phaseRunning, 0)
	at(stepEngine, phaseRunning, 1) // still running: nothing ends
	at(stepMake, phaseRunning, 2.5)
	// Sign is never seen running: it ends with no time of its own.
	at(stepCheck, phaseRunning, 4)
	at(stepCheck, phaseBuilt, 4.6)
	for _, c := range []struct {
		k    stepKind
		want time.Duration
	}{
		{stepEngine, 2500 * time.Millisecond},
		{stepMake, 1500 * time.Millisecond},
		{stepSign, 0},
		{stepCheck, 600 * time.Millisecond},
	} {
		if got := tr.took(c.k); got != c.want {
			t.Errorf("took(%v) = %v, want %v", c.k, got, c.want)
		}
	}
	// A later observation never moves a finished step's end.
	at(stepCheck, phaseBuilt, 9)
	if got := tr.took(stepCheck); got != 600*time.Millisecond {
		t.Errorf("check end moved: %v", got)
	}
	if secs(2100*time.Millisecond) != "2.1s" {
		t.Errorf("secs = %q", secs(2100*time.Millisecond))
	}
}

func TestStepFold(t *testing.T) {
	P, A, D := kit.StepPending, kit.StepActive, kit.StepDone
	if n := foldAt([]kit.StepState{D, D, A, P}, false); n != 0 {
		t.Errorf("two done fold %d, want 0", n)
	}
	if n := foldAt([]kit.StepState{D, D, D, A, P}, false); n != 3 {
		t.Errorf("three done fold %d, want 3", n)
	}
	if n := foldAt([]kit.StepState{D, D, D, A}, true); n != 0 {
		t.Errorf("opened summary folds %d, want 0", n)
	}
}

func TestPlainErrorAndFixes(t *testing.T) {
	a := &App{}
	if got := plainError(&domain.Error{Code: "NODE_NOT_FOUND", Message: "exec: node: not found"}); got != T("fail.node") {
		t.Errorf("NODE_NOT_FOUND said %q", got)
	}
	if got := plainError(&domain.Error{Code: "WHATEVER", Message: "Something odd."}); got != "Something odd." {
		t.Errorf("unknown code said %q", got)
	}
	fx := a.fixesFor(&domain.Error{Code: "NODE_NOT_FOUND"}, "p1", func() {})
	if len(fx) != 2 || fx[0].label != T("projectBuild.getNode") || fx[1].label != T("action.retry") {
		t.Errorf("NODE_NOT_FOUND fixes %+v", fx)
	}
	if fx := a.fixesFor(&domain.Error{Code: "SIGNING_FAILED"}, "p1", nil); len(fx) != 0 {
		t.Errorf("SIGNING_FAILED with no retry: %d fixes", len(fx))
	}
}

func TestBuiltPhase(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		installed, auto, opened, installs bool
		want                              stepPhase
	}{
		{"auto install under way", false, true, false, true, phaseInstalling},
		{"auto install done, not opened yet", true, true, false, true, phaseInstalling},
		{"auto installed and opened", true, true, true, true, phaseInstalled},
		{"installed by hand", true, false, false, true, phaseInstalled},
		{"auto installed, then uninstalled", false, true, true, true, phaseNotInstalled},
		{"never installed", false, false, false, true, phaseNotInstalled},
		{"Install & Open off", false, false, false, false, phaseBuilt},
	} {
		if got := builtPhase(tc.installed, tc.auto, tc.opened, tc.installs); got != tc.want {
			t.Errorf("%s: builtPhase = %v, want %v", tc.name, got, tc.want)
		}
	}
	// Uninstalled after its auto Install & Open: the list gives way to the
	// result bar instead of spinning on Install & Open.
	plan := stepPlan(false, false, true)
	if !stepsDone(stepStates(plan, stepCheck, builtPhase(false, true, true, true))) {
		t.Error("an uninstalled build's steps should be done")
	}
}
