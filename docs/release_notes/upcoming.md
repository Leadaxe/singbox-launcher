# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights

- **AmneziaWG obfuscation is now editable on an existing node.** Open a WireGuard node in Sources and the Settings tab carries an obfuscation block right below Detour: the masquerade domain and browser (`id` / `ib`) plus the junk counters `jc` / `jmin` / `jmax`. Until now those fields only ever arrived with the import — a server that started demanding obfuscation after the node was created meant going back to the provider for a fresh link. Edits are written into the node as you type. The block is hidden when a detour is set, because a node dialled through another outbound raises no transport of its own: choosing a detour clears the fields from the node, and a node that arrives carrying both shows a warning instead of the form. The masquerade protocol is fixed to `quic`; `s1`-`s4` and `h1`-`h4` are never written, since any value there other than the WireGuard default stops the server from recognising the handshake.

### Technical / Internal

- **A pasted wg-quick config no longer loses its masquerade silently.** The INI reader carried the junk numbers and the explicit `i1`-`i5` tags but skipped `ip` / `id` / `ib`, so a config with masquerade looked fully configured while its first decoy packet went out undisguised. The same sugar now also counts as an AmneziaWG marker on its own, which clamps such a node's MTU to the AmneziaWG ceiling — without that, the endpoint fails the silent way: the handshake completes and no data flows.

## RU
### Основное

- **Обфускация AmneziaWG правится у уже заведённого узла.** Откройте WireGuard-узел в Sources — на вкладке Settings сразу под Detour появился блок обфускации: домен и браузер маскировки (`id` / `ib`) плюс junk-числа `jc` / `jmin` / `jmax`. Раньше эти поля приезжали только с импортом, и сервер, начавший требовать обфускацию после заведения узла, отправлял пользователя к провайдеру за новой ссылкой. Правки пишутся в узел по мере ввода. При выбранном detour блок скрыт: узел, ходящий через чужой outbound, своего транспорта не поднимает — назначенный detour снимает поля с узла, а узел, приехавший сразу с обоими, показывает предупреждение вместо формы. Протокол маскировки закреплён как `quic`; `s1`–`s4` и `h1`–`h4` не пишутся вовсе, потому что любое значение там, кроме дефолтного для WireGuard, лишает сервер возможности узнать рукопожатие.

### Техническое / Внутреннее

- **Вставленный wg-quick конфиг больше не теряет маскировку молча.** Разбор INI переносил junk-числа и явные теги `i1`–`i5`, но пропускал `ip` / `id` / `ib`: конфиг с маскировкой выглядел полностью настроенным, а первый decoy-пакет уходил без неё. Тот же сахар теперь сам по себе считается признаком AmneziaWG, из-за чего MTU такого узла ужимается до потолка AmneziaWG — без этого узел отказывает молча: рукопожатие проходит, данные не идут.
