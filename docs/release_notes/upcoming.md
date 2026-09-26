# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

## EN
### Highlights
- Xray subscriptions: ClientHello fragmentation written as `streamSettings.finalmask.tcp` (`type: fragment`) now turns on TLS fragmentation for the node, the same as the older `dialerProxy` → freedom form. Empty `tcpSettings` and `mode`/`path`/`host` repeated inside XHTTP `extra` no longer produce "unknown field" notes.

### Technical / Internal
- Contract 1.1.81: which fields of a route or DNS rule count as match conditions is now registry data. A preset rule left with no condition after substitution is dropped with `template_fragment_dropped`, including a DNS rule that has only an `action`. `wg://` links are accepted, like on LxBox.
- Contract 1.1.82: an empty match field is not a condition. A preset rule whose every `rule_set` reference is missing is dropped whole instead of matching on its remaining conditions.
- Contract 1.1.83: registry `json.` paths accept an array element selector `name[key=value]`; an empty object or array is not reported as an unread field.

## RU
### Основное
- Подписки Xray: фрагментация ClientHello в форме `streamSettings.finalmask.tcp` (`type: fragment`) теперь включает TLS-фрагментацию узла, как и старая форма `dialerProxy` → freedom. Пустой `tcpSettings` и `mode`/`path`/`host`, продублированные в `extra` у XHTTP, больше не дают пометок «неизвестное поле».

### Техническое / Внутреннее
- Контракт 1.1.81: какие поля правила маршрута и DNS считаются условиями, теперь задаёт реестр. Правило пресета, у которого после подстановки не осталось условий, выпадает с `template_fragment_dropped`, в том числе DNS-правило с одним `action`. Ссылки `wg://` принимаются, как у LxBox.
- Контракт 1.1.82: пустое поле-условие условием не считается. Правило пресета, у которого все ссылки `rule_set` висячие, выпадает целиком, а не срабатывает по оставшимся условиям.
- Контракт 1.1.83: пути `json.` в реестре принимают селектор элемента массива `имя[ключ=значение]`; пустой объект или массив не считается непрочитанным полем.
