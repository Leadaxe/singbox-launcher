//go:build windows && !386

package core

import "errors"

// Daemon-режим на Windows (SPEC 141): общий код менеджера уже собирается
// здесь (daemon_manager.go, backend_daemon.go, debugapi_wiring_daemon.go),
// а платформенного слоя службы ещё нет. Ниже — временные заглушки точек
// расширения; движок закрыт daemonEngineAvailable, поэтому в рантайме до них
// доходит только «службы нет».
//
// TODO(SPEC 141 этап 4): имя службы SCM, рендер команды для runas
// (ShellExecuteEx), `sc.exe start sing-box-lxd`, show-secret, каталог
// <ProgramData>\sing-box-lxd; гейт — по версии ядра (≥ 1.14.2-lx.2), а не
// безусловный отказ.

// errDaemonWindowsPending — ответ заглушек, пока нет ядра со службой Windows.
var errDaemonWindowsPending = errors.New("daemon mode on Windows arrives with core v1.14.2-lx.2")

// daemonFallbackRuntimeDir — TODO(SPEC 141 этап 4): <ProgramData>\sing-box-lxd
// через windows.KnownFolderPath (FOLDERID_ProgramData).
const daemonFallbackRuntimeDir = ""

// daemonEngineAvailable — TODO(SPEC 141 этап 4): пока отказ всегда, и
// лаунчер на Windows остаётся на classic.
func daemonEngineAvailable() error { return errDaemonWindowsPending }

// daemonServiceCommand — TODO(SPEC 141 этап 4): рендер {Binary, Args} для
// runas и для Copy (§5.1).
func daemonServiceCommand(_ string, _ ...string) string { return "" }

// daemonBootstrapCommand — TODO(SPEC 141 этап 4): NotRunning лечится
// `sc.exe start sing-box-lxd` через runas.
func daemonBootstrapCommand() string { return "" }

// DaemonKickstartCommand — на Windows команды нет (SPEC 141 §4): install и
// copy сами перезапускают службу.
func (ac *AppController) DaemonKickstartCommand() string { return "" }

// DaemonShowSecretCommand — TODO(SPEC 141 этап 4): daemon.json лежит в
// <ProgramData>\sing-box-lxd (SYSTEM + Administrators).
func (ac *AppController) DaemonShowSecretCommand() string { return "" }
