# Linux: DNS для TUN без запросов пароля (systemd-resolved + Polkit)

На Linux-десктопе с `systemd-resolved` старт TUN-соединения заставляет sing-box настраивать
DNS для TUN-интерфейса через `resolvectl`. Каждый такой вызов — отдельный метод D-Bus к
`systemd-resolved`, и Polkit авторизует каждый по отдельности: один запуск VPN даёт **три
запроса пароля**, остановка — **ещё один**.

Четыре задействованных действия Polkit:

| Действие | Когда |
| --- | --- |
| `org.freedesktop.resolve1.set-dns-servers` | старт |
| `org.freedesktop.resolve1.set-domains` | старт |
| `org.freedesktop.resolve1.set-default-route` | старт |
| `org.freedesktop.resolve1.revert` | остановка |

Проблему нашёл и решил автор issue #126 на openSUSE Tumbleweed; правило не зависит от
дистрибутива — нужен только Polkit с поддержкой JS-правил (`polkit >= 0.106`, то есть любой
современный десктопный дистрибутив).

## Почему привычные способы не помогают

- **`CAP_NET_ADMIN` на бинаре sing-box.** Capability позволяет процессу работать с сетевыми
  интерфейсами напрямую, но `resolvectl` их не трогает: он просит сделать работу
  `systemd-resolved` через D-Bus. Проверка прав происходит на **стороне сервиса**, в Polkit, а
  Polkit смотрит, кто просит, а не какие у него capabilities.
- **Группы `systemd-resolve` / `systemd-network`.** Это служебные группы аккаунтов сервиса: им
  принадлежат файлы и runtime-состояние демона. В политике Polkit для `resolve1` они не
  упоминаются, поэтому вступление в них ничего не меняет.
- **`wheel`.** Членство в `wheel` означает «может аутентифицироваться как администратор» —
  Polkit всё равно спросит пароль, просто примет ваш собственный вместо рутового. Запросы
  останутся.
- **Широкие группы авторизации** (`empower` в openSUSE и аналоги в других дистрибутивах)
  запросы уберут, но дают куда больше прав, чем нужно sing-box. Так делать не стоит.

Лаунчер это правило **не устанавливает**. Это изменение системной конфигурации, поэтому оно
остаётся осознанным и задокументированным действием администратора.

## Установка

### 1. Создать группу и войти в неё

```bash
sudo groupadd --system singbox-dns
sudo usermod --append --groups singbox-dns "$USER"
```

### 2. Установить правило Polkit

```bash
sudo tee /etc/polkit-1/rules.d/49-singbox-resolved.rules >/dev/null <<'EOF'
/*
 * Allow active local members of singbox-dns to configure per-link DNS
 * through systemd-resolved for sing-box TUN connections.
 */
polkit.addRule(function(action, subject) {
    var singBoxResolvedAction =
        action.id == "org.freedesktop.resolve1.set-dns-servers" ||
        action.id == "org.freedesktop.resolve1.set-domains" ||
        action.id == "org.freedesktop.resolve1.set-default-route" ||
        action.id == "org.freedesktop.resolve1.revert";

    if (singBoxResolvedAction &&
        subject.isInGroup("singbox-dns") &&
        subject.local &&
        subject.active) {
        return polkit.Result.YES;
    }
});
EOF

sudo chown root:root /etc/polkit-1/rules.d/49-singbox-resolved.rules
sudo chmod 0644 /etc/polkit-1/rules.d/49-singbox-resolved.rules
```

### 3. Выйти из сессии и зайти снова

Дополнительные группы навешиваются на сессию в момент её создания, поэтому новое членство
доедет до лаунчера только после **полного** выхода из сеанса рабочего стола и входа заново.
Перезапуска самого лаунчера недостаточно.

## Проверка

Убедиться, что группа попала в сессию:

```bash
id
```

В выводе должна быть `singbox-dns`. Если её нет — вы всё ещё в старой сессии, выйдите ещё раз.

Убедиться, что Polkit разобрал правило и знает действия:

```bash
pkaction --action-id org.freedesktop.resolve1.set-dns-servers --verbose
```

Синтаксическую ошибку в файле правил сообщает демон Polkit:

```bash
journalctl -u polkit -n 50
```

Затем запустите и остановите VPN — оба действия должны пройти без единого запроса пароля.

## О безопасности

Правило намеренно узкое:

- действует только на членов выделенной группы `singbox-dns` — по умолчанию не затрагивает
  никого;
- требует **локальной активной** сессии (`subject.local && subject.active`), то есть не
  распространяется на SSH-сессии и на пользователя, от которого переключились;
- разрешает ровно четыре действия `systemd-resolved`, нужные жизненному циклу TUN, а для всего
  остального не возвращает ничего (решение отдаётся политике по умолчанию);
- не даёт ни администраторских прав, ни общих сетевых привилегий — член группы может задать
  DNS для линка, и только.

Добавить ещё одного пользователя:

```bash
sudo usermod --append --groups singbox-dns USERNAME
```

## Удаление

```bash
sudo rm /etc/polkit-1/rules.d/49-singbox-resolved.rules
sudo gpasswd --delete "$USER" singbox-dns
sudo groupdel singbox-dns
```

После этого выйдите из сессии и зайдите снова, чтобы группа ушла из сеанса. Удаление файла
правил Polkit подхватывает сразу — перезапуск демона не нужен.

## См. также

- [docs/BUILD_LINUX.ru.md](BUILD_LINUX.ru.md) — сборка и запуск на Linux, `setcap` для TUN.
- English version: [LINUX_DNS_POLKIT.md](LINUX_DNS_POLKIT.md)
