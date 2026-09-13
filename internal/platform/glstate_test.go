package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/constants"
)

// writeFile — вспомогалка: создать файл с содержимым (директории по пути уже
// должны существовать или создаются здесь же).
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestGLStateLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if _, ok := LoadGLState(dir); ok {
		t.Fatal("LoadGLState on an empty dir must report ok=false")
	}

	want := GLState{
		Phase:             GLPhaseRendered,
		Mode:              GLModeMesa,
		Driver:            "llvmpipe",
		Renderer:          "llvmpipe (LLVM 22.1.8, 256 bits)",
		OfferedHWRenderer: "NVIDIA GeForce RTX 3060",
	}
	if err := SaveGLState(dir, want); err != nil {
		t.Fatalf("SaveGLState: %v", err)
	}

	got, ok := LoadGLState(dir)
	if !ok {
		t.Fatal("LoadGLState after SaveGLState must report ok=true")
	}
	if got.Phase != want.Phase || got.Mode != want.Mode || got.Driver != want.Driver ||
		got.Renderer != want.Renderer || got.OfferedHWRenderer != want.OfferedHWRenderer {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}

	// Битый JSON эквивалентен отсутствию файла: гейт обязан переспросить
	// железо, а не строить решения на мусоре.
	writeFile(t, GLStatePath(dir), "{ this is not json")
	if _, ok := LoadGLState(dir); ok {
		t.Fatal("a corrupt gl-state.json must report ok=false")
	}
}

func TestGLStateMarkStartingKeepsPreviousRendererAndOffer(t *testing.T) {
	dir := t.TempDir()
	prev := GLState{
		Phase:             GLPhaseRendered,
		Mode:              GLModeMesa,
		Renderer:          "llvmpipe (LLVM 22.1.8, 256 bits)",
		Driver:            "llvmpipe",
		OfferedHWRenderer: "Intel(R) UHD Graphics 630",
	}
	if err := SaveGLState(dir, prev); err != nil {
		t.Fatalf("SaveGLState: %v", err)
	}

	MarkGLStarting(dir, GLModeMesa)

	got, ok := LoadGLState(dir)
	if !ok {
		t.Fatal("state must exist after MarkGLStarting")
	}
	if got.Phase != GLPhaseStarting {
		t.Errorf("phase = %q, want %q", got.Phase, GLPhaseStarting)
	}
	// Потеря OfferedHWRenderer означала бы, что диалог возврата на железо
	// всплывает каждый старт, хотя пользователь уже ответил «Later».
	if got.OfferedHWRenderer != prev.OfferedHWRenderer {
		t.Errorf("offered_hw_renderer lost: %q, want %q", got.OfferedHWRenderer, prev.OfferedHWRenderer)
	}
	if got.Renderer != prev.Renderer {
		t.Errorf("renderer lost: %q, want %q", got.Renderer, prev.Renderer)
	}
	if got.Driver != prev.Driver {
		t.Errorf("driver lost: %q, want %q", got.Driver, prev.Driver)
	}

	MarkGLRendered(dir)
	got, _ = LoadGLState(dir)
	if got.Phase != GLPhaseRendered {
		t.Errorf("phase after MarkGLRendered = %q, want %q", got.Phase, GLPhaseRendered)
	}
	if got.OfferedHWRenderer != prev.OfferedHWRenderer || got.Mode != GLModeMesa {
		t.Errorf("MarkGLRendered clobbered the record: %+v", got)
	}
}

func TestDecideGate(t *testing.T) {
	okProbe := probeResult{Major: 4, Minor: 6, Renderer: "NVIDIA GeForce RTX 3060"}
	timeoutProbe := probeResult{Timeout: true}
	refusedProbe := probeResult{Major: 1, Minor: 1, Renderer: "GDI Generic"}

	cases := []struct {
		name string
		in   gateInput
		want gateAction
	}{
		// Фаза «пробовать или нет».
		{"first start: no state file", gateInput{Interactive: true}, actProbe},
		{"previous start rendered on hardware",
			gateInput{HasState: true, State: GLState{Phase: GLPhaseRendered, Mode: GLModeHardware}, Interactive: true}, actStart},
		{"previous start rendered via mesa",
			gateInput{HasState: true, State: GLState{Phase: GLPhaseRendered, Mode: GLModeMesa}, MesaInstalled: true, Interactive: true}, actStart},
		{"previous start died before the first frame",
			gateInput{HasState: true, State: GLState{Phase: GLPhaseStarting, Mode: GLModeMesa}, MesaInstalled: true, Interactive: true}, actProbe},

		// Фаза «что делать с результатом пробы».
		{"probe ok, no mesa around",
			gateInput{Interactive: true, Probed: true, Probe: okProbe}, actStart},
		{"probe ok, mesa is in use → offer to switch back",
			gateInput{Interactive: true, MesaInstalled: true, Probed: true, Probe: okProbe}, actAskDisableMesa},
		{"timeout, interactive",
			gateInput{Interactive: true, Probed: true, Probe: timeoutProbe}, actAskTimeout},
		{"refused, no mesa → offer install",
			gateInput{Interactive: true, Probed: true, Probe: refusedProbe}, actAskInstallMesa},
		{"refused, mesa installed, previous start died under it → nothing works",
			gateInput{Interactive: true, MesaInstalled: true, PrevDiedUnderMesa: true, Probed: true, Probe: refusedProbe}, actNeitherWorks},

		// Целевой сценарий Mesa: RDP/ВМ без GPU. Железа нет и быть не должно,
		// Mesa исправна — ни диалога, ни ошибки, просто старт.
		{"first start after an upgrade: mesa installed by an older version, no hardware (RDP)",
			gateInput{Interactive: true, MesaInstalled: true, Probed: true, Probe: refusedProbe}, actStart},
		{"first start: mesa installed and hardware works → offer to switch back",
			gateInput{Interactive: true, MesaInstalled: true, Probed: true, Probe: okProbe}, actAskDisableMesa},

		// -tray: ни одного диалога ни в одной ветке (критерий приёмки №9).
		{"tray: probe ok with mesa installed",
			gateInput{MesaInstalled: true, Probed: true, Probe: okProbe}, actStart},
		{"tray: timeout",
			gateInput{Probed: true, Probe: timeoutProbe}, actStart},
		{"tray: refused without mesa",
			gateInput{Probed: true, Probe: refusedProbe}, actStart},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideGate(tc.in); got != tc.want {
				t.Errorf("decideGate = %v, want %v", got, tc.want)
			}
		})
	}

	// Отдельная проверка инварианта на всех комбинациях: без interactive
	// «спросить» невозможно в принципе.
	for _, probe := range []probeResult{okProbe, timeoutProbe, refusedProbe} {
		for _, mesa := range []bool{false, true} {
			for _, hasState := range []bool{false, true} {
				for _, died := range []bool{false, true} {
					in := gateInput{
						HasState: hasState, State: GLState{Phase: GLPhaseStarting},
						MesaInstalled: mesa, PrevDiedUnderMesa: died,
						Probed: true, Probe: probe,
					}
					switch decideGate(in) {
					case actAskDisableMesa, actAskTimeout, actAskInstallMesa:
						t.Errorf("non-interactive gate asked a question: %+v", in)
					}
				}
			}
		}
	}
}

func TestDisableEnableMesa(t *testing.T) {
	t.Run("three DLLs there and back", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range mesaDLLs {
			writeFile(t, filepath.Join(dir, name), name+" body")
		}
		if !IsMesaInstalled(dir) {
			t.Fatal("IsMesaInstalled must be true with opengl32 + libgallium_wgl present")
		}

		if err := DisableMesa(dir); err != nil {
			t.Fatalf("DisableMesa: %v", err)
		}
		if IsMesaInstalled(dir) {
			t.Error("Mesa still reported as installed after DisableMesa")
		}
		if !IsMesaDisabled(dir) {
			t.Error("IsMesaDisabled must be true after DisableMesa")
		}
		for _, name := range mesaDLLs {
			if _, err := os.Stat(filepath.Join(dir, name+constants.MesaDisabledSuffix)); err != nil {
				t.Errorf("%s%s is missing: %v", name, constants.MesaDisabledSuffix, err)
			}
		}

		if err := EnableMesa(dir); err != nil {
			t.Fatalf("EnableMesa: %v", err)
		}
		if !IsMesaInstalled(dir) || IsMesaDisabled(dir) {
			t.Error("EnableMesa did not restore the DLLs")
		}
		for _, name := range mesaDLLs {
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if string(body) != name+" body" {
				t.Errorf("%s content changed: %q", name, body)
			}
		}
	})

	t.Run("foreign lone opengl32.dll is left alone", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "opengl32.dll")
		writeFile(t, path, "someone else's driver")

		if IsMesaInstalled(dir) {
			t.Error("a lone opengl32.dll must not count as our Mesa")
		}
		err := DisableMesa(dir)
		if !errors.Is(err, errForeignOpenGL) {
			t.Fatalf("DisableMesa err = %v, want errForeignOpenGL", err)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil || string(body) != "someone else's driver" {
			t.Errorf("the foreign DLL was touched: %q, %v", body, readErr)
		}
	})

	t.Run("EnableMesa without .off copies from the bundle", func(t *testing.T) {
		dir := t.TempDir()
		bundle := filepath.Join(dir, constants.MesaBundleDirName)
		writeFile(t, filepath.Join(bundle, "opengl32.dll"), "bundled opengl32")
		writeFile(t, filepath.Join(bundle, "libgallium_wgl.dll"), "bundled gallium")
		// Не-DLL в папке копироваться не должен.
		writeFile(t, filepath.Join(bundle, "README.txt"), "not a dll")

		if !HasMesaBundle(dir) {
			t.Fatal("HasMesaBundle must be true")
		}
		if err := EnableMesa(dir); err != nil {
			t.Fatalf("EnableMesa: %v", err)
		}
		if !IsMesaInstalled(dir) {
			t.Fatal("EnableMesa did not install from the bundle")
		}
		if _, err := os.Stat(filepath.Join(dir, "README.txt")); err == nil {
			t.Error("a non-DLL was copied out of the bundle")
		}
		body, err := os.ReadFile(filepath.Join(dir, "opengl32.dll"))
		if err != nil || string(body) != "bundled opengl32" {
			t.Errorf("copied opengl32.dll = %q, %v", body, err)
		}
	})

	t.Run("the bundle never overwrites a foreign opengl32.dll", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "opengl32.dll"), "someone else's driver")
		writeFile(t, filepath.Join(dir, constants.MesaBundleDirName, "opengl32.dll"), "bundled opengl32")
		writeFile(t, filepath.Join(dir, constants.MesaBundleDirName, "libgallium_wgl.dll"), "bundled gallium")

		if err := EnableMesa(dir); !errors.Is(err, errForeignOpenGL) {
			t.Fatalf("EnableMesa err = %v, want errForeignOpenGL", err)
		}
		body, err := os.ReadFile(filepath.Join(dir, "opengl32.dll"))
		if err != nil || string(body) != "someone else's driver" {
			t.Errorf("the foreign DLL was overwritten: %q, %v", body, err)
		}
	})
}
