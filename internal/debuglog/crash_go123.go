//go:build go1.23

package debuglog

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

// EnableCrashOutput дублирует трассу фатальной паники в файл path (Go ≥ 1.23,
// runtime/debug.SetCrashOutput). Нужен для windowsgui-сборки: у процесса нет
// stderr, и паника на старте выглядит как «окно мелькнуло и пропало» без
// единой строки в обычном логе. Файл открывается на дозапись — история
// падений накапливается; runtime держит собственную копию дескриптора,
// поэтому наш закрывается сразу.
func EnableCrashOutput(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	err = debug.SetCrashOutput(f, debug.CrashOptions{})
	// Свой дескриптор закрываем сразу: runtime уже держит собственную копию,
	// и в этот файл мы больше не пишем — ошибка закрытия read-only для нас
	// ничего не меняет, но ошибку SetCrashOutput она затирать не должна.
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// releaseCrashOutput снимает дублирование трассы паники в crash.log и
// закрывает копию дескриптора, которую держит runtime: иначе на Windows
// файл (и LogDir) нельзя удалить до выхода процесса.
func releaseCrashOutput() {
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
}
