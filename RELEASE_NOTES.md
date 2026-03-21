# Release Notes

Полный черновик следующего релиза: [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

---

### Выжимка (RU)

- **Визард и главное окно**  
  Отступ под скролл (Rules, Sources, DNS — только список серверов, вкладка **Servers** — внутри строки прокси). Rules: порядок ↑/↓, сохранение позиции скролла, удаление с подтверждением; обновление outbound не сбрасывает несохранённые правки. Sources: компактно «подпись + копирование». Тихий sync при смене вкладок и корректный **hasChanges**, в том числе после правок списка Outbounds.

- **Вкладка DNS**  
  `dns.servers`, `dns.rules` одним JSON `{"rules":[...]}`, final, strategy, кэш, default domain resolver. Состояние в **`dns_options`** в `state.json`. Чекбоксы **enabled**, скелетные строки из шаблона, тултипы, частичный refresh селектов вместо полной пересборки списка.

- **Парсер и sing-box**  
  VLESS / Trojan / VMess: транспорты и TLS из URI; для Xray `xtls-rprx-vision-udp443` в сгенерированном JSON — vision и при необходимости `packet_encoding`. SOCKS5: `socks5://` и `socks://` → в конфиге `type: socks`, `version: "5"`, при наличии в URI — `username` / `password`. Подписка: `tag_prefix` из `#fragment` в URL. UTF-8 (обрезка по рунам), нормализация тегов вроде `❯` → ` > `.

- **Clash API**  
  Percent-encode имён прокси/групп в delay и switch (исправление 404 на сложных тегах).

- **Сборка**  
  Linux: проверка зависимостей, [docs/BUILD_LINUX.md](docs/BUILD_LINUX.md), опциональный Docker. macOS: `build_darwin.sh` (`-i`, `arm64`, справка).

- **Шаблон визарда**  
  Переработан блок DNS в `bin/wizard_template.json`; рекомендуется сбросить сохранённый шаблон в каталоге данных приложения.

- **Внутреннее**  
  `MergeGUIToModel`, виджет `NewCheckWithContent`, обновления документации и локалей.

### Draft highlights (EN)

- **Wizard & UI:** Scrollbar gutters; Rules / Sources / DNS UX; DNS tab with JSON `dns.rules`, `dns_options` in state, enabled servers, tooltips, faster DNS-related updates.
- **Unsaved changes:** Quieter tab sync; Outbounds list correctly marks config dirty after edits.
- **Parser:** VLESS/Trojan/VMess transports & TLS from URI; vision-udp443 → sing-box–compatible JSON; SOCKS5 with credentials and `version: "5"`; subscription `#fragment` → `tag_prefix`; UTF-8 and tag normalization.
- **Clash API:** Encoded proxy/group names in API paths.
- **Build:** Linux dependency checks + docs + optional Docker; macOS script options (`-i`, `arm64`).
- **Template:** Wizard DNS defaults reworked — see full draft.

*Details:* [docs/release_notes/upcoming.md](docs/release_notes/upcoming.md)

---

## Последний релиз / Latest release

| Версия | Описание |
|--------|----------|
| **v0.8.4** | [docs/release_notes/0-8-4.md](docs/release_notes/0-8-4.md) |
| **v0.8.3** | [docs/release_notes/0-8-3.md](docs/release_notes/0-8-3.md) |
| **v0.8.2** | [docs/release_notes/0-8-2.md](docs/release_notes/0-8-2.md) |
| **v0.8.1** | [docs/release_notes/0-8-1.md](docs/release_notes/0-8-1.md) |
| **v0.8.0** | [docs/release_notes/0-8-0.md](docs/release_notes/0-8-0.md) |

Полное описание каждой версии — по ссылке в таблице (full details in linked files).
