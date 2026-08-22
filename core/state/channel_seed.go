package state

// Сидирование каналов из шаблона (SPEC 104).

import "strconv"

// ChannelSeed — стартовый канал шаблона в форме, не зависящей от пакета
// template (state его не импортирует — это нижний слой).
type ChannelSeed struct {
	Tag     string
	Label   string
	Enabled bool
}

// SeedChannels заполняет секцию каналов стартовым набором.
//
// Возвращает true, если секция была засеяна. Сидирование происходит РОВНО
// ОДИН РАЗ — когда секции ещё нет (nil). Непустой слайс — набор
// пользователя; пустой непустой — тоже его выбор («удалил все каналы»), и
// повторный seed вернул бы удалённое на следующем старте.
func (s *State) SeedChannels(seeds []ChannelSeed) bool {
	if s == nil || s.Channels != nil {
		return false
	}
	s.Channels = make([]Channel, 0, len(seeds))
	for _, seed := range seeds {
		if seed.Tag == "" {
			continue
		}
		s.Channels = append(s.Channels, Channel{
			Tag:                       seed.Tag,
			Label:                     seed.Label,
			Enabled:                   seed.Enabled,
			InterruptExistConnections: true,
		})
	}
	return true
}

// NextChannelTag выдаёт свободный тег `vpn-N`.
//
// Ищет первую свободную позицию, а не «максимум + 1»: после удаления
// среднего канала номера не должны уползать вверх, иначе через десяток
// правок пользователь упрётся в потолок при трёх живых каналах.
func NextChannelTag(channels []Channel) string {
	used := make(map[string]bool, len(channels))
	for _, c := range channels {
		used[c.Tag] = true
	}
	for i := 1; i <= MaxChannels; i++ {
		tag := ChannelTagPrefix + strconv.Itoa(i)
		if !used[tag] {
			return tag
		}
	}
	return ""
}

// FindChannel возвращает канал по тегу.
func FindChannel(channels []Channel, tag string) (Channel, bool) {
	for _, c := range channels {
		if c.Tag == tag {
			return c, true
		}
	}
	return Channel{}, false
}

// ChannelTags возвращает теги каналов, включая парные auto-группы включённых
// каналов с автовыбором. Используется как список валидных целей правил.
func ChannelTags(channels []Channel) []string {
	out := make([]string, 0, len(channels)*2)
	for _, c := range channels {
		if c.Tag == "" {
			continue
		}
		out = append(out, c.Tag)
		if c.Auto != nil {
			out = append(out, c.AutoTag())
		}
	}
	return out
}
