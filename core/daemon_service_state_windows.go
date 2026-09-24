//go:build windows && !386

package core

// Платформенная часть классификатора службы на Windows (SPEC 141 §6) —
// временные заглушки: до этапа 4 служба на Windows «не установлена», а
// ключ файла не считается (кэши sha и версии пусты).
//
// TODO(SPEC 141 этап 4): раскладка <ProgramFiles>\sing-box-lxd и
// <ProgramData>\sing-box-lxd (windows.KnownFolderPath), определение службы
// через SCM (BinaryPathName, DACL службы), звено цепочки — владелец и DACL
// по списку разрешённых SID, reparse, NTFS; ключ файла —
// GetFileInformationByHandle; состояние — QueryServiceStatus; сайдкар —
// sing-box-lxd.install.json по набору файлов.

func systemDaemonServiceLayout() daemonServiceLayout { return daemonServiceLayout{} }

func daemonServiceCorePath() string { return "" }

func daemonServiceSidecarPath(_ string) string { return "" }

func inspectDaemonServiceDefinition(_ daemonServiceLayout) DaemonServiceCheck {
	return DaemonServiceCheck{State: DaemonServiceNotInstalled, Detail: errDaemonWindowsPending.Error()}
}

func checkRootOwnedEntry(_ string, _ uint32, _ bool) error { return errDaemonWindowsPending }

// fileHashKey — TODO(SPEC 141 этап 4): (VolumeSerialNumber, FileIndex, size,
// mtime) из GetFileInformationByHandle.
type fileHashKey struct{}

func statHashKey(_ string) (fileHashKey, error) { return fileHashKey{}, errDaemonWindowsPending }

func compareDaemonServiceRunning(_ *DaemonServiceCheck) {}

// legacyCopyRemoveCommand — ранней раскладки lx.11 на Windows не было:
// LegacyPath пуст, и сюда не доходят.
func legacyCopyRemoveCommand(_ string) string { return "" }
