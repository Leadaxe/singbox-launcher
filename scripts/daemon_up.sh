#!/bin/bash
# daemon_up.sh — поднять daemon-режим одним запуском, БЕЗ участия GUI.
#
# Делает всё последовательно:
#   1. сносит старую службу демона + её state (чистим прошлые попытки);
#   2. ставит службу заново (чистый серверный сертификат);
#   3. сопрягает лаунчер по mTLS (enroll клиентского сертификата);
#   4. берёт config.json из бандла, готовит его как это делает лаунчер
#      (убирает clash_api — всё по gRPC, абсолютизирует cache.db),
#      применяет к демону → ядро поднимается;
#   5. пишет итог в ~/daemon_up.txt.
#
# Рабочий VPN НЕ трогает (демон Clash не занимает — clash_api удалён).
# Запуск:  bash scripts/daemon_up.sh    (спросит sudo — установка службы)
#
# После успеха просто открой /Applications/singbox-launcher.app —
# он подхватит сопряжение из настроек и покажет ядро в daemon-режиме.
set -u

APP=/Applications/singbox-launcher.app/Contents/MacOS
CORE="$APP/bin/sing-box"
SUPPORT="/Library/Application Support/sing-box-lxd"
LISTEN="127.0.0.1:9091"
OUT="$HOME/daemon_up.txt"
: > "$OUT"
log() { echo "$@" | tee -a "$OUT"; }

log "=== daemon up $(date) ==="
log "core: $("$CORE" version 2>/dev/null | head -1)"

# --- 1. снос старой службы + state ---
log ""
log "--- 1. remove old service + state ---"
sudo "$CORE" lxd --service=uninstall --purge >>"$OUT" 2>&1
sudo launchctl bootout system/com.leadaxe.sing-box-lxd 2>/dev/null
sleep 1

# --- 2. секрет + установка службы (TLS) ---
log ""
log "--- 2. install service ---"
SECFILE="$APP/bin/daemon/secret"
mkdir -p "$APP/bin/daemon"
if [ ! -s "$SECFILE" ]; then
  head -c 32 /dev/urandom | xxd -p | tr -d '\n' > "$SECFILE"; echo >> "$SECFILE"
  chmod 600 "$SECFILE"
fi
SECRET=$(tr -d '[:space:]' < "$SECFILE")
sudo "$CORE" lxd --service=install --tls --listen "$LISTEN" --secret-file "$SECFILE" >>"$OUT" 2>&1
log "waiting for daemon on 9091..."
for i in $(seq 1 30); do nc -z -w1 127.0.0.1 9091 2>/dev/null && { log "daemon up"; break; }; sleep 0.5; done

# --- 3. сопряжение (mint invite + enroll), сохранение в settings.json лаунчера ---
log ""
log "--- 3. pair (mTLS enroll) ---"
INVITE=$(curl -sk --max-time 5 -X POST "https://$LISTEN/admin/client-code" \
  -H "Authorization: Bearer $SECRET" -H "Content-Type: application/json" -d '{"name":"launcher"}' \
  | sed -n 's/.*"invite":"\([^"]*\)".*/\1/p')
log "invite: $INVITE"
FP=$(echo "$INVITE" | cut -d'#' -f2); CODE=$(echo "$INVITE" | cut -d'#' -f3)
CDIR="$APP/bin/daemon"     # кладём клиентскую пару туда, где её ждёт лаунчер
openssl ecparam -name prime256v1 -genkey -noout -out "$CDIR/client_key.pem" 2>/dev/null
openssl req -new -x509 -key "$CDIR/client_key.pem" -out "$CDIR/client_cert.pem" -days 3650 -subj "/CN=singbox-launcher" 2>/dev/null
chmod 600 "$CDIR/client_key.pem" "$CDIR/client_cert.pem"
ENROLL=$(python3 -c "import json;print(json.dumps({'code':'$CODE','name':'singbox-launcher','cert_pem':open('$CDIR/client_cert.pem').read()}))")
log "enroll:"
curl -sk --max-time 5 --cert "$CDIR/client_cert.pem" --key "$CDIR/client_key.pem" \
  -X POST "https://$LISTEN/admin/enroll" -H "Content-Type: application/json" -d "$ENROLL" 2>&1 | tee -a "$OUT"
log ""

# записываем сопряжение в settings.json лаунчера (адрес/секрет/отпечаток/режим)
python3 - "$APP/bin/settings.json" "$LISTEN" "$FP" "$SECRET" <<'PY'
import json,sys
p=sys.argv[1]
try: d=json.load(open(p))
except: d={}
d['daemon_address']=sys.argv[2]
d['daemon_server_fingerprint']=sys.argv[3]
d['daemon_secret']=sys.argv[4]
d['core_backend_mode']='daemon'
json.dump(d,open(p,'w'),ensure_ascii=False,indent=2)
print("settings.json updated: mode=daemon, fp",sys.argv[3][:12])
PY

# --- 4. apply config.json (убрать clash_api, cache abs) → ядро вверх ---
log ""
log "--- 4. apply config (no clash_api, abs cache) ---"
CFG="$APP/bin/config.json"
if [ ! -f "$CFG" ]; then
  log "!!! config.json not found — open the launcher and rebuild config first"
else
  PREP=$(mktemp)
  python3 - "$CFG" "$SUPPORT/cache.db" > "$PREP" <<'PY'
import json,sys,re
raw=open(sys.argv[1],encoding='utf-8').read()
# config.json — JSONC: содержит блочные маркеры /* @ParserSTART */ и т.п.
# Вырезаем ТОЛЬКО /* ... */ (не трогаем //: в конфиге его нет как комментария,
# зато есть http:// внутри URL — regex по // покалечил бы значения).
# strict=False разрешает control-символы внутри строк (табы в описаниях).
raw=re.sub(r'/\*.*?\*/','',raw,flags=re.S)
d=json.loads(raw, strict=False)
exp=d.get('experimental',{})
exp.pop('clash_api',None)                       # ← Clash убран (gRPC)
if 'cache_file' in exp: exp['cache_file']['path']=sys.argv[2]
json.dump(d,sys.stdout)
PY
  SZ=$(wc -c < "$PREP" | tr -d ' ')
  log "prepared config: $SZ bytes (clash_api removed)"
  curl -sk --max-time 30 --cert "$CDIR/client_cert.pem" --key "$CDIR/client_key.pem" \
    -H "Authorization: Bearer $SECRET" -H "Content-Type: application/json" \
    --data-binary @"$PREP" -X POST "https://$LISTEN/admin/apply" 2>&1 | tee -a "$OUT"
  log ""
  rm -f "$PREP"
fi

# --- 5. статус + порты ---
log ""
log "--- 5. status ---"
curl -sk --max-time 5 --cert "$CDIR/client_cert.pem" --key "$CDIR/client_key.pem" \
  -H "Authorization: Bearer $SECRET" "https://$LISTEN/admin/status" 2>&1 | tee -a "$OUT"
log ""
log "=== ports (9091 daemon; core inbounds; 9090 = your working VPN, untouched) ==="
netstat -an -p tcp 2>/dev/null | grep LISTEN | grep -E "\.(909[0-1]|1080|207[0-9]) " | tee -a "$OUT"

log ""
log "=== DONE. Now open /Applications/singbox-launcher.app — it will show the daemon core running. ==="
echo ""
echo ">>> Готово. Результат в $OUT"
echo ">>> Теперь открой /Applications/singbox-launcher.app — ядро уже поднято в демоне."
