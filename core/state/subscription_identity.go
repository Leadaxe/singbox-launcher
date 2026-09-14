// File subscription_identity.go — чем подписка представляется провайдеру.
//
// Один объект вместо четырёх плоских ключей записи (SPEC 127 §6.0): это ОДНА
// настройка из нескольких частей, и в состоянии, и в бэкапе 1.0 она хранится
// одинаково — бэкап становится сериализацией состояния без маппера.
//
// Дом типа — здесь, а не в core/backup: состояние первично, а формат файла
// теперь его повторяет. core/backup пользуется этим типом через state.
package state

import (
	"encoding/json"
	"sort"
)

// SubscriptionIdentity — UA, идентификатор устройства и режим его отправки.
//
// Все поля — указатели, включая строки: у этой настройки «не задано» и
// «задано пустым» значат разное. Пустой user_agent — это «слать дефолт
// приложения», а отсутствие ключа — «настройки нет вовсе»; сторона,
// применившая пустую строку как значение, затёрла бы свой дефолт. У булевых
// полей то же самое обязательно: nil = «как в системе», false = «явно не
// отправлять».
//
// DeviceOS/VerOS/DeviceModel лаунчер не применяет (per-source их у него нет)
// и не пишет — они в контракте ради LxBox-стороны; приехав в файле, они дают
// backup_source_identity_dropped, как любой неприменённый ключ.
type SubscriptionIdentity struct {
	UserAgent       *string `json:"user_agent,omitempty"`
	SendHWID        *bool   `json:"send_hwid,omitempty"`
	HWID            *string `json:"hwid,omitempty"`
	DeviceOS        *string `json:"device_os,omitempty"`
	VerOS           *string `json:"ver_os,omitempty"`
	DeviceModel     *string `json:"device_model,omitempty"`
	HashDeviceModel *bool   `json:"hash_device_model,omitempty"`

	// presentKeys — какие ключи реально стояли во входном объекте, в порядке
	// объявления в контракте. Нужны ровно для одного: перечислить в
	// предупреждении те, что лаунчер не применил. Без этого списка пришлось
	// бы либо гадать по значениям (не отличив «не было ключа» от «был
	// пустым»), либо разбирать объект вторым проходом по сырому JSON.
	//
	// Общий обход неизвестных ключей (backup.scanUnknown) внутрь identity не
	// спускается намеренно, иначе одна потеря давала бы два предупреждения:
	// своё и backup_unknown_field.
	presentKeys []string
}

// identityKeyOrder — ключи объекта в порядке контракта. Порядок фиксирован, а
// не взят из обхода map: перечень в предупреждении обязан быть
// воспроизводимым, иначе два импорта одного файла дают разный текст.
var identityKeyOrder = []string{
	"user_agent", "send_hwid", "hwid",
	"device_os", "ver_os", "device_model", "hash_device_model",
}

// identityAppliedKeys — то, что лаунчер умеет применить. Остальное (включая
// незнакомое) отбрасывается с backup_source_identity_dropped.
var identityAppliedKeys = map[string]bool{
	"user_agent": true, "send_hwid": true, "hwid": true, "hash_device_model": true,
}

// UnmarshalJSON — обычный разбор плюс запоминание СОСТАВА ключей.
//
// Свой разбор здесь потому, что стандартный теряет разницу между
// отсутствующим ключом и ключом-пустышкой на уровне, который нужен для текста
// предупреждения: указатели различают это для четырёх применяемых полей, но
// про неизвестные ключи в структуре не остаётся ничего.
//
// `null` в поле — это «ключа нет»: объект остаётся нулевым и без состава.
func (i *SubscriptionIdentity) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	type plain SubscriptionIdentity
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*i = SubscriptionIdentity(p)
	if raw == nil {
		return nil
	}
	// Сначала известные ключи в порядке контракта, затем чужие — в
	// лексикографическом: у map порядка нет, а текст обязан быть стабильным.
	for _, k := range identityKeyOrder {
		if _, ok := raw[k]; ok {
			i.presentKeys = append(i.presentKeys, k)
		}
	}
	var extra []string
	for k := range raw {
		if !identityKeyInSchema(k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	i.presentKeys = append(i.presentKeys, extra...)
	return nil
}

// identityKeyInSchema — объявлен ли ключ контрактом.
func identityKeyInSchema(key string) bool {
	for _, k := range identityKeyOrder {
		if k == key {
			return true
		}
	}
	return false
}

// UnappliedKeys — ключи, приехавшие во входном объекте, но лаунчером не
// применяемые: mobile-only тройка device_os/ver_os/device_model и всё
// незнакомое. Порядок — как в presentKeys, то есть воспроизводимый.
func (i *SubscriptionIdentity) UnappliedKeys() []string {
	if i == nil {
		return nil
	}
	var out []string
	for _, k := range i.presentKeys {
		if !identityAppliedKeys[k] {
			out = append(out, k)
		}
	}
	return out
}

// MarkPresentKeys — объявить состав ключей у объекта, собранного кодом, а не
// разбором JSON (импорт легаси-формата 0.x, где ключи читались вручную).
func (i *SubscriptionIdentity) MarkPresentKeys(keys []string) {
	if i == nil {
		return
	}
	i.presentKeys = append([]string(nil), keys...)
}

// IsEmpty — объект не несёт ни одного заданного ключа: хранить и писать его
// незачем (экспорт — чистая функция состояния, П1: пустышка в каждом файле
// была бы шумом, отличающим два одинаковых состояния).
func (i *SubscriptionIdentity) IsEmpty() bool {
	if i == nil {
		return true
	}
	return i.UserAgent == nil && i.SendHWID == nil && i.HWID == nil &&
		i.DeviceOS == nil && i.VerOS == nil && i.DeviceModel == nil &&
		i.HashDeviceModel == nil
}

// Clone — глубокая копия: у записи состояния и у записи файла не должно быть
// общих указателей, иначе правка одной молча меняет другую.
func (i *SubscriptionIdentity) Clone() *SubscriptionIdentity {
	if i == nil {
		return nil
	}
	out := &SubscriptionIdentity{presentKeys: append([]string(nil), i.presentKeys...)}
	out.UserAgent = cloneStrPtr(i.UserAgent)
	out.HWID = cloneStrPtr(i.HWID)
	out.DeviceOS = cloneStrPtr(i.DeviceOS)
	out.VerOS = cloneStrPtr(i.VerOS)
	out.DeviceModel = cloneStrPtr(i.DeviceModel)
	out.SendHWID = cloneBoolPtr(i.SendHWID)
	out.HashDeviceModel = cloneBoolPtr(i.HashDeviceModel)
	return out
}

func cloneStrPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := *v
	return &s
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	b := *v
	return &b
}

// ── доступ по отдельным настройкам ──────────────────────────────────
//
// Читателям (fetcher, UI, экспорт) нужна не структура, а ответ на вопрос
// «что задано у ЭТОЙ подписки»; хелперы ниже дают его, не заставляя каждого
// читателя знать про указатели и про nil-объект.

// IdentityUserAgent — UA подписки; пусто = «как в системе».
func (s *Source) IdentityUserAgent() string {
	if s == nil || s.Identity == nil || s.Identity.UserAgent == nil {
		return ""
	}
	return *s.Identity.UserAgent
}

// IdentityHWID — HWID подписки; пусто = «как в системе».
func (s *Source) IdentityHWID() string {
	if s == nil || s.Identity == nil || s.Identity.HWID == nil {
		return ""
	}
	return *s.Identity.HWID
}

// IdentitySendHWID — отправлять ли X-Hwid; nil = «как в системе».
func (s *Source) IdentitySendHWID() *bool {
	if s == nil || s.Identity == nil {
		return nil
	}
	return s.Identity.SendHWID
}

// IdentityHashDeviceModel — хэшировать ли модель устройства; nil = «как в
// системе».
func (s *Source) IdentityHashDeviceModel() *bool {
	if s == nil || s.Identity == nil {
		return nil
	}
	return s.Identity.HashDeviceModel
}

// SetIdentityUserAgent / SetIdentityHWID / SetIdentitySendHWID /
// SetIdentityHashDeviceModel — правка ОДНОЙ настройки объекта.
//
// Пустая строка (у UA/HWID) и nil (у флагов) означают «как в системе» и
// стирают ключ: в UI это ровно то же действие, что поставить галку
// переопределения обратно. Когда после правки не остаётся ни одного ключа,
// объект снимается целиком — пустышка в подписке отличала бы два одинаковых
// состояния (П1).
func (s *Source) SetIdentityUserAgent(v string) {
	s.editIdentity(func(id *SubscriptionIdentity) { id.UserAgent = optStr(v) })
}

func (s *Source) SetIdentityHWID(v string) {
	s.editIdentity(func(id *SubscriptionIdentity) { id.HWID = optStr(v) })
}

func (s *Source) SetIdentitySendHWID(v *bool) {
	s.editIdentity(func(id *SubscriptionIdentity) { id.SendHWID = cloneBoolPtr(v) })
}

func (s *Source) SetIdentityHashDeviceModel(v *bool) {
	s.editIdentity(func(id *SubscriptionIdentity) { id.HashDeviceModel = cloneBoolPtr(v) })
}

// editIdentity — общая обвязка правки: объект заводится по требованию и
// снимается, когда опустел.
func (s *Source) editIdentity(edit func(*SubscriptionIdentity)) {
	if s == nil {
		return
	}
	cur := s.Identity
	if cur == nil {
		cur = &SubscriptionIdentity{}
	}
	edit(cur)
	if cur.IsEmpty() {
		s.Identity = nil
		return
	}
	s.Identity = cur
}

func optStr(v string) *string {
	if v == "" {
		return nil
	}
	s := v
	return &s
}

// SetIdentity — записать четвёрку применяемых лаунчером настроек.
//
// Mobile-only ключи (device_os/ver_os/device_model) сеттер не трогает: он
// правит ЧЕТВЁРКУ и о них ничего не знает. Это не «провоз» — в состоянии их
// не заводит никто: оба входа бэкапа собирают объект из применённых ключей
// заново и называют остальные вслух (BACKUP.md §«subscriptions[].identity»:
// ключи, которых сторона не применяет, отбрасываются с
// backup_source_identity_dropped). Так что поле, оказавшееся здесь непустым,
// пришло бы не из файла, а из правки состояния мимо импорта.
//
// Пустая строка UA/HWID означает «как в системе» и стирает ключ — то же, что
// у одиночных сеттеров.
func (s *Source) SetIdentity(ua, hwid string, sendHWID, hashModel *bool) {
	s.editIdentity(func(id *SubscriptionIdentity) {
		id.UserAgent = optStr(ua)
		id.HWID = optStr(hwid)
		id.SendHWID = cloneBoolPtr(sendHWID)
		id.HashDeviceModel = cloneBoolPtr(hashModel)
	})
}
