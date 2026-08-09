//go:build darwin

package platform

import (
	"fmt"
	"os"
	"syscall"
)

// privExecFlag — маркер режима привилегированного хелпера. Аргументы после
// него (до `--`) служебные, после `--` — целевая программа и её argv.
const privExecFlag = "--priv-exec"

// MaybeRunPrivExecHelper обрабатывает режим привилегированного exec-хелпера и
// НЕ возвращается, если он активен (передаёт управление целевой программе).
//
// Зачем: AuthorizationExecuteWithPrivileges (macOS Security.framework) даёт
// запускаемому процессу effective uid = 0, но REAL uid остаётся
// пользовательским — execve не меняет real uid. Целевые программы вроде
// `sing-box lxd --service=install` проверяют os.Getuid() (real) и падают с
// "needs root". Хелпер запускается САМИМ лаунчером под привилегиями (euid=0),
// вызывает setuid(0) — при euid=0 это поднимает и real, и saved uid до 0 —
// и exec'ает цель, которая теперь видит настоящий root.
//
// Формат: <launcher> --priv-exec <outPath> <donePath> -- <program> [args...]
// Хелпер выполняет program, пишет combined output в outPath, exit-код в
// donePath (маркер завершения для ожидающего родителя).
func MaybeRunPrivExecHelper() {
	args := os.Args
	if len(args) < 2 || args[1] != privExecFlag {
		return // обычный запуск GUI
	}
	// args: [self, --priv-exec, out, done, --, program, a1, a2, ...]
	if len(args) < 6 || args[4] != "--" {
		fmt.Fprintln(os.Stderr, "priv-exec: bad arguments")
		os.Exit(2)
	}
	outPath, donePath := args[2], args[3]
	target := args[5]
	targetArgs := args[5:]

	// Поднимаем real uid до 0. При euid=0 (нас запустил AEWP) это обязано
	// пройти; иначе — не под привилегиями, отказываемся.
	if err := syscall.Setuid(0); err != nil {
		writeDone(outPath, donePath, 3, "priv-exec: setuid(0) failed: "+err.Error())
		os.Exit(3)
	}

	// Открываем out-файл для combined stdout/stderr цели. Права 0644, НЕ 0600:
	// хелпер работает от root (после setuid), а маркеры out/done читает
	// лаунчер от имени пользователя. Root-owned 0600-файл пользователь
	// прочитать не может (не владелец) — это вешало RunPrivilegedProgramAndWait
	// на таймаут: done-маркер физически есть, но недоступен ожидающему
	// родителю. temp-каталог всё равно 0700 (owner=пользователь), так что
	// снаружи файлы не видны — 0644 внутри приватного каталога безопасно.
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		writeDone(outPath, donePath, 4, "priv-exec: open out: "+err.Error())
		os.Exit(4)
	}

	// Запускаем цель как обычный дочерний процесс (не exec: нам нужно поймать
	// exit-код и записать done-маркер после завершения).
	proc, err := os.StartProcess(target, targetArgs, &os.ProcAttr{
		Files: []*os.File{nil, outFile, outFile},
		Sys:   &syscall.SysProcAttr{},
	})
	if err != nil {
		outFile.Close()
		writeDone(outPath, donePath, 5, "priv-exec: start target: "+err.Error())
		os.Exit(5)
	}
	state, waitErr := proc.Wait()
	outFile.Close()
	code := 0
	if waitErr != nil {
		code = 6
	} else {
		code = state.ExitCode()
	}
	writeDoneCode(donePath, code)
	os.Exit(code)
}

func writeDone(outPath, donePath string, code int, msg string) {
	_ = os.WriteFile(outPath, []byte(msg+"\n"), 0o644)
	writeDoneCode(donePath, code)
}

// writeDoneCode пишет done-маркер 0644: его читает лаунчер-родитель от имени
// пользователя, а мы — root; 0600 сделал бы маркер нечитаемым и подвесил бы
// ожидание (см. комментарий у outFile).
func writeDoneCode(donePath string, code int) {
	_ = os.WriteFile(donePath, []byte(fmt.Sprintf("%d", code)), 0o644)
}
