# Release Notes

## v0.6.1

<details>
<summary><b>🇷🇺 Русская версия / Russian Version</b></summary>

### 🐛 Исправления ошибок

#### Clash API
- **Исправлена проблема с перечитыванием конфигурации Clash API**: Теперь конфигурация API перечитывается из config.json при каждом запуске sing-box (Start), что позволяет подхватывать изменения, сделанные через визард
- **Улучшены сообщения об ошибках подключения**: На Windows теперь показывается понятное сообщение с предложением подождать 15 секунд и попробовать снова при ошибках подключения к Clash API

#### Технические улучшения
- Добавлен метод `ReloadClashAPIConfig()` в `APIService` для перечитывания конфигурации из файла
- Улучшена обработка ошибок подключения к Clash API с учетом платформы

</details>

<details>
<summary><b>🇬🇧 English Version</b></summary>

### 🐛 Bug Fixes

#### Clash API
- **Fixed Clash API config reload issue**: API configuration is now reloaded from config.json on every sing-box start, allowing to pick up changes made via wizard
- **Improved connection error messages**: On Windows, a clear message with suggestion to wait 15 seconds and try again is shown for Clash API connection errors

#### Technical Improvements
- Added `ReloadClashAPIConfig()` method to `APIService` for reloading configuration from file
- Improved Clash API connection error handling with platform detection

</details>

---

См. [docs/release_notes/0-6-0.md](docs/release_notes/0-6-0.md) для подробностей о предыдущем релизе.

See [docs/release_notes/0-6-0.md](docs/release_notes/0-6-0.md) for details about the previous release.
