package tabs

import (
	"strings"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestInterfaceHintExplainsEmptyValue(t *testing.T) {
	// Пустое значение = ключ default_interface не пишется вовсе. Это штатный
	// режим, и подпись обязана говорить именно это, а не молчать и не пугать.
	got := interfaceHintFor(&wizardmodels.WizardModel{}, "", map[string]string{})
	if got == "" || strings.Contains(got, "⚠") {
		t.Fatalf("подпись для пустого значения = %q, ожидалось нейтральное пояснение", got)
	}
}

func TestInterfaceHintDescribesKnownInterface(t *testing.T) {
	hints := map[string]string{"en0": "en0 — Wi-Fi (192.168.10.124)"}
	if got := interfaceHintFor(&wizardmodels.WizardModel{}, "en0", hints); got != hints["en0"] {
		t.Fatalf("подпись = %q, ожидалась расшифровка из списка", got)
	}
}

func TestInterfaceHintWarnsAboutUnknownLocalName(t *testing.T) {
	// Опечатка в имени = ядро стартует, но трафика нет. Молчать здесь нельзя.
	got := interfaceHintFor(&wizardmodels.WizardModel{}, "definitely-no-such-iface-42", map[string]string{})
	if !strings.Contains(got, "⚠") {
		t.Fatalf("подпись = %q, ожидалось предупреждение", got)
	}
}

func TestInterfaceHintDoesNotWarnOnRemoteTarget(t *testing.T) {
	// Имя относится к другой машине: сверять его с локальными интерфейсами
	// нельзя, иначе валидное имя роутера показывалось бы как ошибка.
	m := &wizardmodels.WizardModel{Target: wizardtemplate.RemoteTarget("linux", "amd64")}
	got := interfaceHintFor(m, "eth0", map[string]string{})
	if strings.Contains(got, "⚠") {
		t.Fatalf("подпись = %q, для remote-таргета предупреждать не о чем", got)
	}
}

func TestInterfacePickLocalNamesAreBare(t *testing.T) {
	// В выпадающем списке SelectEntry лежит то, что уедет в конфиг дословно:
	// имя обязано быть чистым, без подписи вида «en0 — Wi-Fi (…)».
	names, hints := interfacePickOptions(&wizardmodels.WizardModel{}, "")
	for _, n := range names {
		if strings.ContainsAny(n, " ()—") {
			t.Errorf("имя %q содержит оформление — оно уедет в конфиг как есть", n)
		}
		if hints[n] == "" {
			t.Errorf("для %q нет расшифровки", n)
		}
	}
}

func TestInterfacePickRemoteWithoutProviderIsEmpty(t *testing.T) {
	// Провайдер не установлен / машина не подключена: подсказок нет, и это
	// рабочее состояние — поле остаётся пригодным для ручного ввода.
	SetRemoteInterfaceProvider(nil)
	m := &wizardmodels.WizardModel{Target: wizardtemplate.RemoteTarget("linux", "amd64")}
	names, _ := interfacePickOptions(m, "")
	if len(names) != 0 {
		t.Fatalf("список = %v, ожидался пустой", names)
	}
}

func TestInterfacePickRemoteUsesProvider(t *testing.T) {
	SetRemoteInterfaceProvider(func(id string) ([]string, map[string]string, bool) {
		if id != "home" {
			t.Errorf("провайдер получил machineID %q, ожидался home", id)
		}
		return []string{"eth0", "wan"}, map[string]string{
			"eth0": "eth0 (192.168.10.1)",
			"wan":  "wan (10.20.30.40)",
		}, true
	})
	defer SetRemoteInterfaceProvider(nil)

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "home"),
	}
	names, hints := interfacePickOptions(m, "")
	if len(names) != 2 || names[0] != "eth0" {
		t.Fatalf("список = %v, ожидались интерфейсы удалённой машины", names)
	}
	if hints["wan"] == "" {
		t.Error("расшифровка удалённого интерфейса потеряна")
	}
	// И подпись обязана брать её же, а не ругаться на «неизвестное имя».
	if got := interfaceHintFor(m, "wan", hints); got != hints["wan"] {
		t.Errorf("подпись = %q, ожидалась расшифровка от демона", got)
	}
}
