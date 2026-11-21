# SPEC 184 — TrustTunnel: подписка и форма узла

**Тип:** F (фича). **Статус:** C. **Контракт:** 1.1.176.
**Решение владельца:** 21.11.2026 — «сделай подписку TrustTunnel и форму Add server» (вторая очередь после ссылки `tt://`, контракт 1.1.175).

## 1. Проблема

1. Ссылка `tt://` бывает указателем на подписку: в ней только адрес, откуда клиент AdGuard забирает параметры сервера. Такая ссылка отклонялась кодом `tt_subscription_only`, узел получить было нельзя.
2. Адрес подписки отвечает объектом JSON одного сервера, а не списком ссылок. Источник с таким URL не давал ни одного узла.
3. Узел TrustTunnel заводился только ссылкой или JSON: в «Add server» его не было.

## 2. Решения

### 2.1 Ответ подписки читается как ссылка

Ключи объекта ответа (`hostname`, `address`, `username`, `password`, `has_ipv6`, `upstream_protocol`, `anti_dpi`, `skip_verification`, `certificate`, `custom_sni`, `client_random_prefix`, `name`, `dns_upstreams`) совпадают с полями контейнера ссылки `tt://`. Второй таблицы записей не заводим: распаковщик уровня документа `container_link` печатает объект ссылкой по диалекту `tlv` секции (кодом обратного хода 1.1.175), и дальше работает уже сделанный разбор. Происхождение узла — ссылка `tt://`; новый вид происхождения, правки бэкапа и состояния не нужны.

Вид источника `trusttunnel_endpoint_json` объявлен в `registry/source_kinds.json`. Путь подписки (`ParseSubscriptionBody`) распаковывает такой документ перед своей классификацией.

### 2.2 Ссылка-указатель становится подпиской

Код `tt_subscription_only` несёт адрес подписки параметром `subscription_url`. При вставке источников (вкладка Sources) строка-указатель уходит в подписки наравне с `https://…`. В списке ссылок и в теле чужой подписки строка остаётся отказом. Логин и пароль из userinfo адреса уходят HTTP Basic Auth — это делает стандартный HTTP-клиент, кода не требуется.

Ссылка с полным узлом и адресом подписки — без изменений: узел из статических полей, код `tt_subscription_ignored`.

### 2.3 Форма

Секция `body.form` реестра, как у naive (SPEC 168): `connection` — Server*, Port*, Username*, Password*; `tls` — SNI, Skip certificate verification, uTLS fingerprint (ALPN и Reality исключены, `tls.enabled` фиксирован); `advanced` — QUIC, Congestion control (при QUIC), Health check, Idle timeout. В списке типов «Add server» — «TrustTunnel» после MASQUE.

## 3. Вне scope

Клиентские настройки AdGuard, которых нет у ядра (`custom_sni`, client random, anti-DPI, запасные адреса, DNS профиля); сертификат и ALPN в форме; подписка с самоподписанным сертификатом узла (загрузка идёт системным хранилищем доверия).

## 4. Проверки

`go build ./...`; `l10n_check --strict`; по именам: `TestContractCorpusBody`, `TestContractCorpusForm`, `TestContractCorpusURI`, `TestSubscriptionURLOfLink`, `TestSourceKinds`, `TestContractMapperSectionsMatchSchema`. UI — без тестов, показ владельцу. Полный прогон — CI `run_mode=tests`.
