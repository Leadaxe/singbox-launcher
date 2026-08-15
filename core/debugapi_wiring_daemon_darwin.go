//go:build darwin

package core

import (
	"fmt"

	"google.golang.org/grpc"

	"singbox-launcher/core/debugapi"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// debugAPIDaemonWiring adapts *AppController to debugapi.DaemonFacade
// (SPEC 100 §3.6). Живёт в core, как и debugAPIFacade, — здесь есть
// конкретные типы и нет цикла импортов.
type debugAPIDaemonWiring struct {
	ac *AppController
}

// debugAPIDaemonFacade — darwin-реализация. Другие платформы (см. стаб)
// возвращают nil, и группа /daemon/* не регистрируется.
func (ac *AppController) debugAPIDaemonFacade() debugapi.DaemonFacade {
	return &debugAPIDaemonWiring{ac: ac}
}

func (f *debugAPIDaemonWiring) Status() debugapi.DaemonStatus {
	st := f.ac.DaemonStatusSnapshot()
	return debugapi.DaemonStatus{
		CoreSupportsLxd:  st.CoreSupportsLxd,
		ServiceInstalled: st.ServiceInstalled,
		Paired:           st.Paired,
		Address:          st.Address,
		Reachable:        st.Reachable,
		CoreStatus:       st.CoreStatus,
		LastError:        st.LastError,
		InterruptedApply: st.InterruptedApply,
		DaemonVersion:    st.DaemonVersion,
		StateDir:         st.StateDir,
	}
}

func (f *debugAPIDaemonWiring) Pair(invite, secret string) error {
	return f.ac.PairDaemonWithInvite(invite, secret)
}

func (f *debugAPIDaemonWiring) Unpair() error { return f.ac.UnpairDaemon() }

func (f *debugAPIDaemonWiring) SetAddress(addr string) error { return f.ac.SetDaemonAddress(addr) }

func (f *debugAPIDaemonWiring) SetSecret(secret string) error { return f.ac.SetDaemonSecret(secret) }

func (f *debugAPIDaemonWiring) Commands() debugapi.DaemonCommands {
	install, err := f.ac.DaemonInstallCommand()
	if err != nil {
		// Единственный источник ошибки — отсутствие пути к ядру; отдать
		// пустую строку честнее, чем валить весь ответ.
		install = ""
	}
	return debugapi.DaemonCommands{
		Install:        install,
		Uninstall:      f.ac.DaemonUninstallCommand(false),
		UninstallPurge: f.ac.DaemonUninstallCommand(true),
		Repair:         f.ac.DaemonRepairCommand(),
		Kickstart:      f.ac.DaemonKickstartCommand(),
		ShowSecret:     f.ac.DaemonShowSecretCommand(),
	}
}

func (f *debugAPIDaemonWiring) EngineMode() string { return string(f.ac.BackendMode()) }

// SwitchEngine — тот же порядок, что радио в conn.local (SPEC 096):
// переключить активный бэкенд, затем персистить выбор. SwitchBackendMode сам
// гейтит работающий VPN и несопряжённый демон.
func (f *debugAPIDaemonWiring) SwitchEngine(mode string) error {
	var m BackendMode
	switch mode {
	case string(BackendClassic):
		m = BackendClassic
	case string(BackendDaemon):
		m = BackendDaemon
	default:
		return fmt.Errorf("unknown engine mode %q", mode)
	}
	if err := f.ac.SwitchBackendMode(m); err != nil {
		return err
	}
	binDir := platform.GetBinDir(f.ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.CoreBackendMode = string(m)
	if err := locale.SaveSettings(binDir, st); err != nil {
		return fmt.Errorf("engine switched, but persisting the choice failed: %w", err)
	}
	return nil
}

func (f *debugAPIDaemonWiring) AdminDo(method, path string, body []byte, contentType string) (int, []byte, string, error) {
	cfg, err := DaemonConfigFromSettings(f.ac)
	if err != nil {
		return 0, nil, "", err
	}
	return lxdclient.New(cfg).Do(method, path, body, contentType)
}

// GRPCConn — соединение per-вызов: loopback-dial дёшев (grpc.NewClient ленив),
// а держать постоянный канал пришлось бы инвалидировать при каждой смене
// адреса/пина/секрета. Закрывает вызывающий (raw-handler).
func (f *debugAPIDaemonWiring) GRPCConn() (*grpc.ClientConn, error) {
	cfg, err := DaemonConfigFromSettings(f.ac)
	if err != nil {
		return nil, err
	}
	return lxdclient.New(cfg).DialGRPC()
}
