# Security policy

[Русская версия ниже](#политика-безопасности).

How the launcher protects the boundary between the user's rights and higher
ones is described in the [security model](docs/SECURITY_MODEL.md).

## Supported versions

Fixes go into the latest release. Before reporting, check that the problem is
present in the [latest release](https://github.com/Leadaxe/singbox-launcher/releases/latest).

## Reporting a vulnerability

Report privately. Do not open a public issue and do not post details in the
Telegram chat until a fix is released.

1. Open the repository's **Security** tab.
2. Choose **Report a vulnerability**.
3. Fill in the form.

Direct link: <https://github.com/Leadaxe/singbox-launcher/security/advisories/new>.

Vulnerabilities in the core are reported in its own repository:
[sing-box-lx security policy](https://github.com/Leadaxe/sing-box-lx/blob/lx/SECURITY.md).
If you are not sure where the problem is, report it here.

### What to include

- Launcher version, core version, operating system and its version.
- Mode: classic or daemon, with TUN or without.
- What an attacker needs at the start: a program under the user's account,
  access to the local network, a subscription server.
- What the attacker gains: higher rights, the user's data, control of the VPN.
- Steps to reproduce. A minimal config helps; remove keys, passwords and
  subscription addresses from it.

### What is a vulnerability here

- A program with the user's rights gains `root`, administrator or `SYSTEM`
  rights through the launcher or the daemon service.
- A privileged process executes a file, reads an environment or follows a path
  that the user's account can change.
- Data from a subscription, a template or a rule set leads to execution of a
  command or to writing outside the launcher's folders.
- Access to the daemon or to the Debug API without a key or a token.
- Traffic leaves past the tunnel when the config says otherwise.

### What is not

- The items listed as accepted risks in the
  [security model](docs/SECURITY_MODEL.md#10-accepted-risks).
- Actions of someone who already has `root` or administrator rights.
- A program under the user's account reading the user's own files.
- A node that does not connect, or a blocked protocol: open a regular
  [issue](https://github.com/Leadaxe/singbox-launcher/issues).

### After the report

The discussion goes on in the private advisory. When the fix is released, the
advisory is published and the reporter is credited, unless they ask otherwise.

---

# Политика безопасности

Как лаунчер защищает границу между правами пользователя и более высокими,
описано в [модели безопасности](docs/SECURITY_MODEL.ru.md).

## Поддерживаемые версии

Исправления выходят в последнем релизе. Перед сообщением проверьте, что
проблема есть в [последнем релизе](https://github.com/Leadaxe/singbox-launcher/releases/latest).

## Как сообщить об уязвимости

Сообщайте приватно. Не открывайте публичный issue и не публикуйте подробности
в Telegram-чате, пока не вышло исправление.

1. Откройте вкладку **Security** репозитория.
2. Выберите **Report a vulnerability**.
3. Заполните форму.

Прямая ссылка: <https://github.com/Leadaxe/singbox-launcher/security/advisories/new>.

Об уязвимостях в ядре сообщайте в его репозитории:
[политика безопасности sing-box-lx](https://github.com/Leadaxe/sing-box-lx/blob/lx/SECURITY.md).
Если неясно, где проблема, сообщайте сюда.

### Что указать

- Версия лаунчера, версия ядра, операционная система и её версия.
- Режим: classic или daemon, с TUN или без.
- Что нужно атакующему на старте: программа под учётной записью пользователя,
  доступ в локальную сеть, сервер подписки.
- Что атакующий получает: высокие права, данные пользователя, управление VPN.
- Шаги воспроизведения. Помогает минимальный конфиг; уберите из него ключи,
  пароли и адреса подписок.

### Что здесь считается уязвимостью

- Программа с правами пользователя получает права `root`, администратора или
  `SYSTEM` через лаунчер или службу демона.
- Привилегированный процесс исполняет файл, читает окружение или идёт по пути,
  которые учётная запись пользователя может изменить.
- Данные подписки, шаблона или набора правил приводят к исполнению команды или
  к записи вне папок лаунчера.
- Доступ к демону или к Debug API без ключа или токена.
- Трафик уходит мимо туннеля, когда конфиг требует обратного.

### Что не считается

- Пункты из списка принятых рисков в
  [модели безопасности](docs/SECURITY_MODEL.ru.md#10-принятые-риски).
- Действия того, у кого уже есть `root` или права администратора.
- Чтение программой под учётной записью пользователя его собственных файлов.
- Узел, который не подключается, или заблокированный протокол: откройте
  обычный [issue](https://github.com/Leadaxe/singbox-launcher/issues).

### После сообщения

Обсуждение идёт в приватном advisory. После выхода исправления advisory
публикуется, автор сообщения указывается, если он не попросит иного.
