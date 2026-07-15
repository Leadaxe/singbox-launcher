// Кеш WARP-регистраций: конверсия между аккаунтами warp и секцией
// state.WarpAccounts, которая живёт в state.json.
//
// Зачем кеш: Cloudflare привязывает выданные адреса к ключу, а каждая
// регистрация — это новый ключ со своим IPv6. Без кеша «Add WARP», нажатый
// дважды (например MASQUE H2 и следом H3), создаёт две независимые регистрации;
// в LxBox на телефоне обе ноды сидят на одной. Кеш повторяет то поведение и
// заодно не плодит device-записи в Cloudflare на каждое открытие визарда.
//
// WG и MASQUE кешируются раздельно: у них разные типы ключей (X25519 против
// ECDSA P-256), одной записью не покрыть.
//
// Из кеша сознательно исключены параметры ноды, а не регистрации: network
// (h2/h3), sni, idle_timeout, keep_alive. Именно поэтому H2 и H3 строятся из
// одной записи — меняется только транспорт.
package warp

import "singbox-launcher/core/state"

// WGToCache — снимок регистрации для сохранения в state.json.
func WGToCache(a *Account) *state.WarpWGAccount {
	if a == nil {
		return nil
	}
	return &state.WarpWGAccount{
		PrivateKey: a.PrivateKey,
		PeerPublic: a.PeerPublic,
		ClientV4:   a.ClientV4,
		ClientV6:   a.ClientV6,
		ClientID:   a.ClientID,
		DeviceID:   a.DeviceID,
		Token:      a.Token,
		AccountID:  a.AccountID,
		License:    a.License,
		WarpPlus:   a.WarpPlus,
		CreatedAt:  a.CreatedAt,
	}
}

// WGFromCache восстанавливает регистрацию из кеша. nil — если записи нет или
// она без ключа/адреса (тогда вызывающий регистрируется заново).
//
// Endpoint и AWG не кешируются: это параметры ноды (пресет обфускации, выбор
// эндпоинта, кубик 🎲), а не регистрации — их задаёт UI при каждой сборке.
func WGFromCache(c *state.WarpWGAccount) *Account {
	if c == nil || c.PrivateKey == "" || c.ClientV4 == "" {
		return nil
	}
	return &Account{
		PrivateKey: c.PrivateKey,
		PeerPublic: c.PeerPublic,
		ClientV4:   c.ClientV4,
		ClientV6:   c.ClientV6,
		ClientID:   c.ClientID,
		DeviceID:   c.DeviceID,
		Token:      c.Token,
		AccountID:  c.AccountID,
		License:    c.License,
		WarpPlus:   c.WarpPlus,
		CreatedAt:  c.CreatedAt,
	}
}

// MasqueToCache — снимок MASQUE-регистрации для state.json.
func MasqueToCache(a *MasqueAccount) *state.WarpMasqueAccount {
	if a == nil {
		return nil
	}
	return &state.WarpMasqueAccount{
		PrivateKeyDER: a.PrivateKeyDER,
		ServerPubDER:  a.ServerPubDER,
		ClientV4:      a.ClientV4,
		ClientV6:      a.ClientV6,
		Server:        a.Server,
		Port:          a.Port,
		DeviceID:      a.DeviceID,
		Token:         a.Token,
		CreatedAt:     a.CreatedAt,
	}
}

// MasqueFromCache восстанавливает MASQUE-регистрацию из кеша. nil — если записи
// нет или в ней нет ключей.
//
// Network/SNI/таймауты не восстанавливаются: их проставляет вызывающий из UI,
// благодаря чему одна регистрация даёт и H2, и H3.
func MasqueFromCache(c *state.WarpMasqueAccount) *MasqueAccount {
	if c == nil || c.PrivateKeyDER == "" || c.ServerPubDER == "" {
		return nil
	}
	return &MasqueAccount{
		PrivateKeyDER: c.PrivateKeyDER,
		ServerPubDER:  c.ServerPubDER,
		ClientV4:      c.ClientV4,
		ClientV6:      c.ClientV6,
		Server:        c.Server,
		Port:          c.Port,
		DeviceID:      c.DeviceID,
		Token:         c.Token,
		CreatedAt:     c.CreatedAt,
	}
}
