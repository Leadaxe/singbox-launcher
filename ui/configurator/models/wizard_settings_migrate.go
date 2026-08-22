package models

import "strings"

// MigrateSettingsVarsFromConfigParams переносит enable_tun_macos → vars.tun при отсутствии tun (идемпотентно).
func MigrateSettingsVarsFromConfigParams(sf *WizardStateFile) {
	if sf == nil {
		return
	}
	for _, v := range sf.Vars {
		if v.Name == "tun" {
			return
		}
	}
	for _, p := range sf.ConfigParams {
		if p.Name == "enable_tun_macos" {
			sf.Vars = append(sf.Vars, PersistedSettingVar{Name: "tun", Value: p.Value})
			return
		}
	}
}

// MigrateTunAddressSplit разводит многострочный `tun_address` по двум полям.
//
// Пока `tun_address` был списком, IPv6-адрес туннеля задавался ВТОРОЙ
// строкой в том же поле — до того, как появилась галка Enable IPv6 со своим
// `tun_address6`. Два способа задать одно и то же сосуществовали, и на
// десктопе поле разъехалось с мобильным (разрыв N7 в contract/registry:
// desktop text_list, mobile text). Теперь оба поля — однострочные, по
// одному адресу на семейство.
//
// Поэтому при загрузке: первая строка остаётся в `tun_address`, первая
// IPv6-строка переезжает в `tun_address6` (если то ещё не задано) и
// включает `ipv6_enabled`. Не отбрасываем: это осмысленная настройка
// пользователя, а не мусор, и перекладывание её в соседнее поле
// однозначно — иначе туннель молча лишился бы IPv6.
//
// Лишние строки сверх первой на семейство отбрасываются: ядро список
// принимает, но интерфейсу клиентского туннеля больше одного адреса на
// семейство не нужно, и хранить невыразимое в форме значение значило бы
// оставить третий способ настройки.
//
// Идемпотентна: однострочный `tun_address` не трогается.
func MigrateTunAddressSplit(sf *WizardStateFile) {
	if sf == nil {
		return
	}
	idx := -1
	for i := range sf.Vars {
		if sf.Vars[i].Name == "tun_address" {
			idx = i
			break
		}
	}
	if idx < 0 || !strings.Contains(sf.Vars[idx].Value, "\n") {
		return
	}

	var first4, first6 string
	for _, line := range strings.Split(sf.Vars[idx].Value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Семейство определяем по двоеточию, а не разбором префикса: в поле
		// может лежать что угодно вплоть до опечатки, и «не IPv6» обязано
		// оставаться в IPv4-поле, где пользователь его и увидит.
		if strings.Contains(line, ":") {
			if first6 == "" {
				first6 = line
			}
			continue
		}
		if first4 == "" {
			first4 = line
		}
	}

	if first4 != "" {
		sf.Vars[idx].Value = first4
	} else {
		// Только IPv6 в поле IPv4 — оставляем первую строку как есть.
		// Затирать в пустоту нельзя: без адреса TUN не поднимется, а
		// пользователь не поймёт, куда делась настройка.
		sf.Vars[idx].Value = strings.TrimSpace(strings.SplitN(sf.Vars[idx].Value, "\n", 2)[0])
	}

	if first6 == "" {
		return
	}
	// Два прохода, а не один: порядок записей в state произвольный, и при
	// однопроходном разборе `ipv6_enabled`, встретившийся РАНЬШЕ
	// `tun_address6`, включался бы даже при уже заданном адресе.
	has6, flagIdx := false, -1
	for i := range sf.Vars {
		switch sf.Vars[i].Name {
		case "tun_address6":
			has6 = strings.TrimSpace(sf.Vars[i].Value) != ""
		case "ipv6_enabled":
			flagIdx = i
		}
	}
	if has6 {
		return // явная настройка пользователя важнее унаследованной строки
	}
	sf.Vars = append(sf.Vars, PersistedSettingVar{Name: "tun_address6", Value: first6})
	if flagIdx >= 0 {
		sf.Vars[flagIdx].Value = "true"
	} else {
		sf.Vars = append(sf.Vars, PersistedSettingVar{Name: "ipv6_enabled", Value: "true"})
	}
}
