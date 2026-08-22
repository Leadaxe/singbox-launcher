package state

import (
	"encoding/json"
	"testing"
)

// Сидирование происходит РОВНО ОДИН РАЗ: непустой набор принадлежит
// пользователю, а пустой непустой — тоже его выбор («удалил все каналы»).
// Повторный seed возвращал бы удалённое на каждом старте.
func TestSeedChannelsOnlyWhenSectionAbsent(t *testing.T) {
	seeds := []ChannelSeed{
		{Tag: "vpn-1", Label: "VPN ①", Enabled: true},
		{Tag: "vpn-2", Label: "VPN ②", Enabled: false},
	}

	fresh := &State{}
	if !fresh.SeedChannels(seeds) {
		t.Fatal("пустое состояние не засеяно")
	}
	if len(fresh.Channels) != 2 {
		t.Fatalf("засеяно %d каналов, ожидалось 2", len(fresh.Channels))
	}
	if !fresh.Channels[0].Enabled || fresh.Channels[1].Enabled {
		t.Errorf("флаги включения не перенесены: %+v", fresh.Channels)
	}
	if !fresh.Channels[0].InterruptExistConnections {
		t.Error("interrupt_exist_connections должен быть включён по умолчанию")
	}

	// Повторный вызов ничего не меняет.
	before := len(fresh.Channels)
	if fresh.SeedChannels(seeds) {
		t.Error("повторное сидирование выполнилось")
	}
	if len(fresh.Channels) != before {
		t.Error("повторное сидирование изменило набор")
	}

	// Пользователь удалил все каналы — это его выбор, не «секции нет».
	emptied := &State{Channels: []Channel{}}
	if emptied.SeedChannels(seeds) {
		t.Error("пустой набор пользователя засеян заново — удалённые каналы вернулись бы")
	}
}

// Тег ищется по первой свободной позиции: после удаления среднего канала
// номера не должны уползать вверх, иначе через десяток правок пользователь
// упрётся в потолок при трёх живых каналах.
func TestNextChannelTagFillsGaps(t *testing.T) {
	channels := []Channel{{Tag: "vpn-1"}, {Tag: "vpn-3"}}
	if got := NextChannelTag(channels); got != "vpn-2" {
		t.Errorf("NextChannelTag = %q, ожидался vpn-2 (дырка)", got)
	}
}

func TestNextChannelTagExhausted(t *testing.T) {
	var channels []Channel
	for i := 0; i < MaxChannels; i++ {
		tag := NextChannelTag(channels)
		if tag == "" {
			t.Fatalf("теги кончились на %d-м из %d", i+1, MaxChannels)
		}
		channels = append(channels, Channel{Tag: tag})
	}
	if got := NextChannelTag(channels); got != "" {
		t.Errorf("выдан тег %q сверх потолка %d", got, MaxChannels)
	}
}

// Канал, записанный без явного "enabled", обязан читаться ВКЛЮЧЁННЫМ:
// нулевое значение bool — false, и без своего размаршалера канал
// самопроизвольно выключался бы при каждом чтении.
func TestChannelUnmarshalDefaults(t *testing.T) {
	var c Channel
	if err := json.Unmarshal([]byte(`{"tag":"vpn-1","label":"Main"}`), &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !c.Enabled {
		t.Error("канал без явного enabled прочитан выключенным")
	}
	if !c.InterruptExistConnections {
		t.Error("interrupt_exist_connections без явного значения прочитан как false")
	}

	// Явный false обязан сохраняться.
	var off Channel
	if err := json.Unmarshal([]byte(`{"tag":"vpn-2","enabled":false}`), &off); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if off.Enabled {
		t.Error("явный enabled:false проигнорирован")
	}
}

func TestChannelAutoTagAndLabel(t *testing.T) {
	c := Channel{Tag: "vpn-3"}
	if got := c.AutoTag(); got != "vpn-3-auto" {
		t.Errorf("AutoTag = %q", got)
	}
	if got := c.DisplayLabel(); got != "vpn-3" {
		t.Errorf("без метки DisplayLabel = %q, ожидался тег", got)
	}
	c.Label = "Германия"
	if got := c.DisplayLabel(); got != "Германия" {
		t.Errorf("DisplayLabel = %q", got)
	}
}

// Теги каналов — список валидных целей правил; auto-группа считается целью
// только когда автовыбор включён (иначе она не эмитится).
func TestChannelTagsIncludeAutoOnlyWhenEnabled(t *testing.T) {
	tags := ChannelTags([]Channel{
		{Tag: "vpn-1"},
		{Tag: "vpn-2", Auto: &ChannelAuto{URL: "http://x/204"}},
	})
	want := map[string]bool{"vpn-1": true, "vpn-2": true, "vpn-2-auto": true}
	if len(tags) != len(want) {
		t.Fatalf("теги %v, ожидалось %d записей", tags, len(want))
	}
	for _, tag := range tags {
		if !want[tag] {
			t.Errorf("лишний тег %q", tag)
		}
	}
	for _, tag := range tags {
		if tag == "vpn-1-auto" {
			t.Error("auto-группа выключенного автовыбора попала в цели")
		}
	}
}

// Каналы переживают запись и чтение состояния; отсутствие секции
// сохраняется как отсутствие (nil), а не превращается в пустой набор —
// иначе сидирование из шаблона не сработает ни разу.
func TestChannelsSurviveDiskRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/state.json"

	src := &State{
		Version: SchemaVersionV6,
		Channels: []Channel{
			{Tag: "vpn-1", Label: "Main", Enabled: true, InterruptExistConnections: true},
			{Tag: "vpn-2", Label: "Backup", Enabled: false, NodeFilter: "^DE-",
				Auto: &ChannelAuto{URL: "http://cp/204", Interval: "5m", Tolerance: 50}},
		},
	}
	if err := src.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Channels) != 2 {
		t.Fatalf("прочитано %d каналов, ожидалось 2", len(got.Channels))
	}
	if got.Channels[1].NodeFilter != "^DE-" || got.Channels[1].Auto == nil {
		t.Errorf("параметры канала потеряны: %+v", got.Channels[1])
	}
	if got.Channels[1].Auto.Tolerance != 50 {
		t.Errorf("tolerance потерян: %+v", got.Channels[1].Auto)
	}
	if got.Channels[1].Enabled {
		t.Error("выключенный канал прочитан включённым")
	}
}

func TestStateWithoutChannelsSectionStaysNil(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/state.json"

	if err := (&State{Version: SchemaVersionV6}).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Channels != nil {
		t.Errorf("секция каналов появилась сама: %v — сидирование из шаблона не сработает", got.Channels)
	}
}
