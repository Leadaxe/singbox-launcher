package core

import (
	"strings"
	"testing"
	"time"

	"singbox-launcher/core/services"
)

// TestServiceRecipes — табличная проверка чистой логики окна Service
// (SPEC 161): сравнение версий ядра, init по платформе, команды рецептов по
// init-системам, ssh-цель и обёртка, классификатор ошибки связи, имя ассета
// ядра под платформу машины.
func TestServiceRecipes(t *testing.T) {
	t.Run("CompareCoreVersion", func(t *testing.T) {
		cases := []struct {
			running, required string
			want              CoreVersionVerdict
		}{
			{"1.14.2-lx.11", "1.14.3-lx.14", CoreVersionOlder},
			{"1.14.3-lx.11", "1.14.3-lx.14", CoreVersionOlder},
			{"1.14.3-lx.14-rc1", "1.14.3-lx.14", CoreVersionOlder},
			{"1.14.2-lx.14", "1.14.3-lx.14", CoreVersionOlder},
			{"1.14.3-lx.14", "1.14.3-lx.14", CoreVersionCurrent},
			{"v1.14.3-lx.14", "1.14.3-lx.14", CoreVersionCurrent},
			{"1.14.3-lx.15", "1.14.3-lx.14", CoreVersionNewer},
			{"unknown", "1.14.3-lx.14", CoreVersionUnknown},
			{"1.12.0", "1.14.3-lx.14", CoreVersionUnknown},
			{"", "1.14.3-lx.14", CoreVersionUnknown},
		}
		for _, c := range cases {
			if got := CompareCoreVersion(c.running, c.required); got != c.want {
				t.Errorf("CompareCoreVersion(%q, %q) = %d, want %d", c.running, c.required, got, c.want)
			}
		}
		labels := []struct{ running, required, a, b string }{
			{"1.14.2-lx.11", "1.14.3-lx.14", "lx.11", "lx.14"},
			{"1.14.2-lx.14", "1.14.3-lx.14", "1.14.2-lx.14", "1.14.3-lx.14"},
			{"1.14.3-lx.14-rc1", "1.14.3-lx.14", "lx.14-rc1", "lx.14"},
			{"unknown", "1.14.3-lx.14", "unknown", "lx.14"},
		}
		for _, c := range labels {
			if a, b := CoreVersionPairLabels(c.running, c.required); a != c.a || b != c.b {
				t.Errorf("CoreVersionPairLabels(%q, %q) = %q, %q; want %q, %q", c.running, c.required, a, b, c.a, c.b)
			}
		}
	})

	t.Run("DefaultServiceInit", func(t *testing.T) {
		cases := []struct {
			goos, goarch string
			want         ServiceInit
		}{
			{"linux", "arm64", ServiceInitProcd},
			{"linux", "arm", ServiceInitProcd},
			{"linux", "mipsle", ServiceInitProcd},
			{"linux", "amd64", ServiceInitSystemd},
			{"darwin", "arm64", ServiceInitLaunchd},
			{"windows", "amd64", ServiceInitSCM},
		}
		for _, c := range cases {
			if got := DefaultServiceInit(c.goos, c.goarch); got != c.want {
				t.Errorf("DefaultServiceInit(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.want)
			}
		}
		if got := NormalizeInitChoice("systemd", "linux", "arm64"); got != ServiceInitSystemd {
			t.Errorf("NormalizeInitChoice(systemd, linux/arm64) = %q, want systemd", got)
		}
		if got := NormalizeInitChoice("procd", "darwin", "arm64"); got != ServiceInitLaunchd {
			t.Errorf("NormalizeInitChoice(procd, darwin) = %q, want launchd", got)
		}
	})

	t.Run("BuildServiceRecipes", func(t *testing.T) {
		yes := true
		reported := ServicePassport{Executable: "/root/sing-box", StateDir: "/etc/sing-box-lxd/state", LogPath: "/tmp/lxd.log", Listen: "192.168.10.1:19091", TLS: &yes}
		router := ServicePlatform{GOOS: "linux", GOARCH: "arm64", Init: ServiceInitProcd}
		vps := ServicePlatform{GOOS: "linux", GOARCH: "amd64", Init: ServiceInitSystemd}
		mac := ServicePlatform{GOOS: "darwin", GOARCH: "arm64", Init: ServiceInitLaunchd}
		win := ServicePlatform{GOOS: "windows", GOARCH: "amd64", Init: ServiceInitSCM}
		root := services.SSHTarget{User: "root", Host: "192.168.10.1"}
		today := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

		withPassport := BuildServiceRecipes(ServiceRecipeInput{Platform: router, Paths: MergeServicePaths(reported, DefaultServicePaths(router)),
			SSH: root, Running: "1.14.2-lx.11", Uploaded: "/Users/me/Downloads/sing-box-1.14.3-lx.14-linux-arm64", Today: today})
		withoutPassport := BuildServiceRecipes(ServiceRecipeInput{Platform: router, Paths: DefaultServicePaths(router),
			SSH: services.SSHTarget{User: "root", Host: "router", Port: 2222}, Running: "unknown", Today: today})
		systemd := BuildServiceRecipes(ServiceRecipeInput{Platform: vps, Paths: DefaultServicePaths(vps),
			SSH: services.SSHTarget{User: "admin", Host: "vps"}, Running: "1.14.2-lx.11", Today: today})
		launchd := BuildServiceRecipes(ServiceRecipeInput{Platform: mac, Paths: DefaultServicePaths(mac)})
		scm := BuildServiceRecipes(ServiceRecipeInput{Platform: win, Paths: DefaultServicePaths(win)})

		cases := []struct {
			name     string
			r        ServiceRecipes
			id       string
			command  string
			defaults bool
		}{
			{"procd status", withPassport, ServiceStepStatus, "/etc/init.d/sing-box-lxd status", false},
			{"procd restart", withPassport, ServiceStepRestart, "/etc/init.d/sing-box-lxd restart", false},
			{"procd log", withPassport, ServiceStepLogTail, "tail -n 100 /tmp/lxd.log", false},
			{"procd log default", withoutPassport, ServiceStepLogTail, "tail -n 100 /tmp/lxd.log", true},
			{"procd last-good", withPassport, ServiceStepLastGood,
				"/etc/init.d/sing-box-lxd stop && /root/sing-box lxd --state-dir /etc/sing-box-lxd/state --config-force /etc/sing-box-lxd/state/last_good.json", false},
			{"procd last-good default", withoutPassport, ServiceStepLastGood,
				"/etc/init.d/sing-box-lxd stop && /usr/bin/sing-box lxd --state-dir /etc/sing-box-lxd/state --config-force /etc/sing-box-lxd/state/last_good.json", true},
			{"procd swap by version", withPassport, ServiceStepCoreSwap,
				"cp /root/sing-box /root/sing-box.1.14.2-lx.11.bak && /etc/init.d/sing-box-lxd stop && mv /tmp/sing-box.new /root/sing-box && chmod 755 /root/sing-box && /etc/init.d/sing-box-lxd start", false},
			{"procd swap by date", withoutPassport, ServiceStepCoreSwap,
				"cp /usr/bin/sing-box /root/sing-box.20261010.bak && /etc/init.d/sing-box-lxd stop && mv /tmp/sing-box.new /usr/bin/sing-box && chmod 755 /usr/bin/sing-box && /etc/init.d/sing-box-lxd start", true},
			{"procd upload", withPassport, ServiceStepCoreUpload,
				"ssh root@192.168.10.1 'cat > /tmp/sing-box.new' < /Users/me/Downloads/sing-box-1.14.3-lx.14-linux-arm64", false},
			{"procd upload port", withoutPassport, ServiceStepCoreUpload,
				"ssh -p 2222 root@router 'cat > /tmp/sing-box.new' < <downloaded sing-box>", false},
			{"procd port owner", withPassport, ServiceStepPortOwner, "netstat -lnp | grep :19091", false},
			{"systemd status", systemd, ServiceStepStatus, "systemctl status sing-box-lxd --no-pager", false},
			{"systemd restart non-root", systemd, ServiceStepRestart, "sudo systemctl restart sing-box-lxd", false},
			{"systemd log non-root", systemd, ServiceStepLogTail, "sudo tail -n 100 /var/lib/sing-box-lxd/lxd.log", true},
			{"systemd swap non-root", systemd, ServiceStepCoreSwap,
				"sudo sh -c 'cp /usr/local/bin/sing-box /usr/local/bin/sing-box.1.14.2-lx.11.bak && systemctl stop sing-box-lxd && mv /tmp/sing-box.new /usr/local/bin/sing-box && chmod 755 /usr/local/bin/sing-box && systemctl start sing-box-lxd'", true},
			{"launchd status", launchd, ServiceStepStatus, "launchctl print system/com.leadaxe.sing-box-lxd", false},
			{"launchd restart", launchd, ServiceStepRestart, "sudo launchctl kickstart -k system/com.leadaxe.sing-box-lxd", false},
			{"launchd log", launchd, ServiceStepLogTail, "sudo tail -n 100 '/Library/Application Support/sing-box-lxd/lxd.log'", true},
			{"launchd last-good", launchd, ServiceStepLastGood,
				"sudo launchctl bootout system/com.leadaxe.sing-box-lxd ; sudo /Library/PrivilegedHelperTools/sing-box-lxd lxd --state-dir '/Library/Application Support/sing-box-lxd/state' --config-force '/Library/Application Support/sing-box-lxd/state/last_good.json'", true},
			{"scm status", scm, ServiceStepStatus, "sc.exe query sing-box-lxd", false},
			{"scm restart", scm, ServiceStepRestart, "Restart-Service -Name sing-box-lxd -Force", false},
			{"scm log", scm, ServiceStepLogTail, `Get-Content -Tail 100 'C:\ProgramData\sing-box-lxd\logs\lxd.log'`, true},
			{"scm last-good", scm, ServiceStepLastGood,
				`sc.exe stop sing-box-lxd; & 'C:\Program Files\sing-box-lxd\sing-box-lxd.exe' lxd --state-dir 'C:\ProgramData\sing-box-lxd\state' --config-force 'C:\ProgramData\sing-box-lxd\state\last_good.json'`, true},
		}
		for _, c := range cases {
			st, ok := c.r.Steps[c.id]
			if !ok {
				t.Errorf("%s: step %q missing", c.name, c.id)
				continue
			}
			if st.Command != c.command {
				t.Errorf("%s:\n got %s\nwant %s", c.name, st.Command, c.command)
			}
			if st.UsesDefault != c.defaults {
				t.Errorf("%s: UsesDefault = %v, want %v", c.name, st.UsesDefault, c.defaults)
			}
		}
		if up := withPassport.Steps[ServiceStepCoreUpload]; !up.RunsLocally || up.Placeholder {
			t.Errorf("core_upload with a file: RunsLocally=%v Placeholder=%v, want true/false", up.RunsLocally, up.Placeholder)
		}
		if up := withoutPassport.Steps[ServiceStepCoreUpload]; !up.Placeholder {
			t.Errorf("core_upload without a file must be a placeholder")
		}
		if withPassport.BackupPath != "/root/sing-box.1.14.2-lx.11.bak" {
			t.Errorf("BackupPath = %q", withPassport.BackupPath)
		}
		if !strings.Contains(withPassport.ScratchScript, "procd_set_param command /root/sing-box lxd --state-dir /etc/sing-box-lxd/state") ||
			withPassport.ScratchPath != "/etc/init.d/sing-box-lxd" {
			t.Errorf("procd scratch script/path wrong: %q\n%s", withPassport.ScratchPath, withPassport.ScratchScript)
		}
		for _, r := range []ServiceRecipes{launchd, scm} {
			if _, ok := r.Steps[ServiceStepCoreSwap]; ok {
				t.Errorf("core_swap must not be built for macOS/Windows")
			}
		}
	})

	t.Run("SSHTarget", func(t *testing.T) {
		cases := []struct {
			in   string
			want services.SSHTarget
			str  string
		}{
			{"root@192.168.10.1", services.SSHTarget{User: "root", Host: "192.168.10.1"}, "root@192.168.10.1"},
			{"u@h:2222", services.SSHTarget{User: "u", Host: "h", Port: 2222}, "u@h:2222"},
			{"h", services.SSHTarget{Host: "h"}, "h"},
			{"[fe80::1]:22", services.SSHTarget{Host: "fe80::1", Port: 22}, "[fe80::1]:22"},
			{"root@fe80::1", services.SSHTarget{User: "root", Host: "fe80::1"}, "root@fe80::1"},
		}
		for _, c := range cases {
			got, err := services.ParseSSHTarget(c.in)
			if err != nil || got != c.want || got.String() != c.str {
				t.Errorf("ParseSSHTarget(%q) = %+v, %v (String %q); want %+v (%q)", c.in, got, err, got.String(), c.want, c.str)
			}
		}
		for _, bad := range []string{"", "@h", "u@", "u@h:0", "u@h:x", "-oProxyCommand=x", "u@h;rm", "[fe80::1", "u v@h"} {
			if _, err := services.ParseSSHTarget(bad); err == nil {
				t.Errorf("ParseSSHTarget(%q) accepted, want error", bad)
			}
		}
		if got := services.DefaultSSHTarget("192.168.10.1:19091"); got.String() != "root@192.168.10.1" {
			t.Errorf("DefaultSSHTarget = %q", got.String())
		}
		target := services.SSHTarget{User: "admin", Host: "vps", Port: 2222}
		if got, want := WrapSSH(target, "echo 'hi'", true), `ssh -t -p 2222 admin@vps 'echo '\''hi'\'''`; got != want {
			t.Errorf("WrapSSH = %s, want %s", got, want)
		}
		if got, want := WrapSSH(services.SSHTarget{User: "root", Host: "r"}, "/etc/init.d/sing-box-lxd status", false), "ssh root@r '/etc/init.d/sing-box-lxd status'"; got != want {
			t.Errorf("WrapSSH = %s, want %s", got, want)
		}
	})

	t.Run("ClassifyDaemonReachError", func(t *testing.T) {
		cases := []struct {
			err  string
			want DaemonReachKind
		}{
			{"", ReachOK},
			{"dial tcp 192.168.10.1:19091: connect: connection refused", ReachDown},
			{"context deadline exceeded", ReachDown},
			{"server fingerprint mismatch: got ab, want cd", ReachCertChanged},
			{"remote error: tls: bad certificate", ReachNotTrusted},
			{"tls: first record does not look like a TLS handshake", ReachChannelMismatch},
			{"HTTP 400: Client sent an HTTP request to an HTTPS server.", ReachChannelMismatch},
			{"Post \"https://h/admin/status\": EOF", ReachConnReset},
			{"something odd", ReachUnknown},
		}
		for _, c := range cases {
			if got := ClassifyDaemonReachError(c.err); got != c.want {
				t.Errorf("ClassifyDaemonReachError(%q) = %d, want %d", c.err, got, c.want)
			}
		}
	})

	t.Run("SingboxAssetSuffixFor", func(t *testing.T) {
		cases := []struct{ goos, goarch, want string }{
			{"linux", "arm64", "linux-arm64.tar.gz"},
			{"linux", "arm", "linux-armv7.tar.gz"},
			{"linux", "mipsle", "linux-mipsle-softfloat.tar.gz"},
			{"linux", "386", ""},
			{"windows", "amd64", "windows-amd64.zip"},
		}
		for _, c := range cases {
			if got := SingboxAssetSuffixFor(c.goos, c.goarch); got != c.want {
				t.Errorf("SingboxAssetSuffixFor(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.want)
			}
		}
	})
}
