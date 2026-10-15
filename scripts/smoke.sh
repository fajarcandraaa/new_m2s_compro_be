#!/usr/bin/env bash
# BE smoke test: exercises every endpoint with curl and prints a markdown
# table (method, path, expected, got). Needs curl, jq and python3.
#
#   B=http://localhost:8080 ADMIN_EMAIL=... ADMIN_PASSWORD=... scripts/smoke.sh
#
# Run it against a freshly seeded database: it creates and then deletes its
# own content, and sends 5 leads from 203.0.113.1 to hit the 5/min limit.
set -u
B=${B:-http://localhost:8080}
ADMIN_EMAIL=${ADMIN_EMAIL:-admin@mindtoscreen.com}
ADMIN_PASSWORD=${ADMIN_PASSWORD:?set ADMIN_PASSWORD}
F=$(mktemp -d)
python3 - "$F" <<'PY'
import os, struct, sys, zlib
d = sys.argv[1]
def png(w, h):
    raw = b"".join(b"\x00" + b"\x30\xc2\xdb" * w for _ in range(h))
    def ch(t, data):
        return struct.pack(">I", len(data)) + t + data + struct.pack(">I", zlib.crc32(t + data) & 0xffffffff)
    return b"\x89PNG\r\n\x1a\n" + ch(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + ch(b"IDAT", zlib.compress(raw)) + ch(b"IEND", b"")
open(os.path.join(d, "logo.png"), "wb").write(png(120, 40))
open(os.path.join(d, "big.png"), "wb").write(png(2, 2) + os.urandom(2 * 1024 * 1024))
open(os.path.join(d, "evil.svg"), "w").write('<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>')
PY
JAR=$(mktemp); EJAR=$(mktemp)
CS='X-Requested-With: m2s-cms'
J='Content-Type: application/json'
PASS=0; FAILN=0
row() { # expected actual method path note
  local mark="ok"; if [ "$1" != "$2" ]; then mark="**FAIL**"; FAILN=$((FAILN+1)); else PASS=$((PASS+1)); fi
  echo "| $3 | \`$4\` | $1 | $2 | $mark | $5 |"
}
# call EXPECT METHOD PATH NOTE [curl args...]  → body in /tmp file $OUT
OUT=$(mktemp)
call() {
  local exp=$1 m=$2 p=$3 note=$4; shift 4
  local code; code=$(curl -s -o "$OUT" -w '%{http_code}' -X "$m" "$B$p" "$@")
  row "$exp" "$code" "$m" "$p" "$note"
}
jqo() { jq -r "$1" "$OUT"; }

echo "| Method | Path | Expected | Got | Result | Note |"
echo "|---|---|---|---|---|---|"
# --- system & public
call 200 GET /healthz "db ok"
call 200 GET /api/v1/site "seeded settings"
call 200 GET /api/v1/services "5 seeded"
call 200 GET "/api/v1/services?audience=personal" "audience filter"
call 400 GET "/api/v1/services?audience=x" "bad enum"
call 200 GET /api/v1/services/web-development "4 packages"
call 404 GET /api/v1/services/tidak-ada "unknown slug"
call 200 GET /api/v1/case-studies ""
call 200 GET "/api/v1/case-studies?featured=true" "max 2"
call 200 GET /api/v1/case-studies/tumbuh-sistem-pertumbuhan-sdm ""
call 200 GET "/api/v1/articles?page=1&pageSize=9" "3 seeded"
call 200 GET "/api/v1/articles?tag=keamanan-data" "2 items"
call 200 GET /api/v1/articles/data-ktp-bocor-itu-sebenarnya-dampaknya-apa ""
call 200 GET "/api/v1/articles/data-ktp-bocor-itu-sebenarnya-dampaknya-apa/related?limit=3" ""
call 200 GET /api/v1/tags "with articleCount"
call 200 GET /api/v1/promos "empty seed"
call 200 GET "/api/v1/testimonials?limit=3" ""
call 200 GET /api/v1/client-logos ""
call 404 GET /api/v1/nope "JSON 404"
call 405 DELETE /api/v1/site "JSON 405"
# --- leads (public)
LEAD='{"name":"Sari","company":"PT Maju","email":"sari@example.com","whatsapp":"0812-3456-7890","service":"custom-software-development","startTimeframe":"1–3 bulan lagi","message":"Kami butuh HRIS untuk 200 karyawan."}'
call 201 POST /api/v1/leads "valid, no CSRF needed" -H "$J" -H 'X-Forwarded-For: 203.0.113.1' -d "$LEAD"
LEAD_ID=$(jqo .data.id)
call 201 POST /api/v1/leads "honeypot → fake 201" -H "$J" -H 'X-Forwarded-For: 203.0.113.1' -d '{"name":"bot","email":"x","whatsapp":"1","message":"","website":"spam"}'
call 422 POST /api/v1/leads "bad whatsapp" -H "$J" -H 'X-Forwarded-For: 203.0.113.1' -d '{"name":"A","email":"a@b.co","whatsapp":"12345","message":"pesan cukup panjang"}'
call 400 POST /api/v1/leads "unknown field" -H "$J" -H 'X-Forwarded-For: 203.0.113.1' -d '{"name":"A","foo":1}'
call 201 POST /api/v1/leads "5th request" -H "$J" -H 'X-Forwarded-For: 203.0.113.1' -d "$LEAD"
call 429 POST /api/v1/leads "6th/min → 429 + Retry-After" -H "$J" -H 'X-Forwarded-For: 203.0.113.1' -d "$LEAD"
# --- auth
call 403 POST /api/v1/auth/login "missing CSRF header" -H "$J" -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"
call 401 POST /api/v1/auth/login "wrong password" -H "$J" -H "$CS" -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"wrong-password\"}"
call 401 GET /api/v1/auth/me "no cookie"
call 200 POST /api/v1/auth/login "sets m2s_session" -H "$J" -H "$CS" -c "$JAR" -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"
call 200 GET /api/v1/auth/me "" -b "$JAR"
A=(-b "$JAR" -H "$CS" -H "$J")
call 403 POST /api/v1/admin/articles "CSRF checked before session" -H "$J" -d '{}'
# --- dashboard & settings
call 200 GET /api/v1/admin/dashboard "" -b "$JAR"
call 200 GET /api/v1/admin/settings "" -b "$JAR"
SET=$(jq -c '.data | del(.updatedAt,.showTestimonials,.showClientLogos) | .whatsappNumber="0822-3010-9923"' "$OUT")
call 200 PUT /api/v1/admin/settings "E.164 normalised; revalidate site" "${A[@]}" -d "$SET"
call 422 PUT /api/v1/admin/settings "javascript: URL" "${A[@]}" -d "$(echo "$SET" | jq -c '.mapsUrl="javascript:alert(1)"')"
call 200 PATCH /api/v1/admin/settings/toggles "partial" "${A[@]}" -d '{"showClientLogos":true}'
call 422 PATCH /api/v1/admin/settings/toggles "{} → 422" "${A[@]}" -d '{}'
call 200 PATCH /api/v1/admin/settings/toggles "restore" "${A[@]}" -d '{"showClientLogos":false}'
# --- services
SVC='{"audience":"business","eyebrow":"","title":"Layanan Smoke","tagline":"","summary":"","points":["A"],"priceLabel":"","priceValue":"","audienceText":"","deliverables":[],"systems":[],"packages":[{"name":"P1","priceLabel":"","priceValue":"Rp 1","description":"","features":["x"],"highlighted":true}],"seo":{"title":null,"description":null,"ogImageUrl":null},"status":"draft"}'
call 200 GET /api/v1/admin/services "unpaginated" -b "$JAR"
call 201 POST /api/v1/admin/services "slug generated, appended" "${A[@]}" -d "$SVC"
SVC_ID=$(jqo .data.id)
call 200 GET /api/v1/admin/services/$SVC_ID "" -b "$JAR"
call 200 PUT /api/v1/admin/services/$SVC_ID "slug kept" "${A[@]}" -d "$(echo "$SVC" | jq -c '.title="Layanan Smoke 2"')"
call 409 POST /api/v1/admin/services "explicit slug taken" "${A[@]}" -d "$(echo "$SVC" | jq -c '.slug="web-development"')"
curl -s -b "$JAR" "$B/api/v1/admin/services" -o "$OUT"; IDS=$(jq -c '{ids:[.data[].id]}' "$OUT")
call 200 PUT /api/v1/admin/services/order "all ids" "${A[@]}" -d "$IDS"
call 422 PUT /api/v1/admin/services/order "missing id" "${A[@]}" -d "$(echo "$IDS" | jq -c '.ids |= .[1:]')"
call 400 GET /api/v1/admin/services/not-a-uuid "bad id" -b "$JAR"
# --- case studies
CSB='{"systemName":"SMOKE","category":"DMS","featured":true,"title":"Studi Smoke","summary":"","cardBody":"","metrics":[{"value":"1","label":"a"},{"value":"2","label":"b"}],"scope":[],"initialConditionsTitle":"","initialConditions":[],"risksTitle":"","risks":[],"approachTitle":"","approach":[],"solutions":[],"impactTitle":"","impacts":[],"lesson":"","seo":{"title":null,"description":null,"ogImageUrl":null},"status":"published"}'
call 200 GET "/api/v1/admin/case-studies?q=TUMBUH" "" -b "$JAR"
call 409 POST /api/v1/admin/case-studies "3rd featured published" "${A[@]}" -d "$CSB"
call 201 POST /api/v1/admin/case-studies "featured draft allowed" "${A[@]}" -d "$(echo "$CSB" | jq -c '.status="draft"')"
CS_ID=$(jqo .data.id)
call 200 GET /api/v1/admin/case-studies/$CS_ID "" -b "$JAR"
call 409 PUT /api/v1/admin/case-studies/$CS_ID "publishing it → 409 featured" "${A[@]}" -d "$CSB"
call 200 PUT /api/v1/admin/case-studies/$CS_ID "unfeatured publish" "${A[@]}" -d "$(echo "$CSB" | jq -c '.featured=false')"
# --- articles
ART='{"title":"Artikel Smoke & Uji","excerpt":"x","contentHtml":"<p onclick=\"x()\">Halo</p><script>alert(1)</script>","status":"published","publishedAt":"2030-01-01T08:00:00+07:00","tags":["Smoke","AI & Teknologi"],"coverImageUrl":null,"seo":{"title":null,"description":null,"ogImageUrl":null}}'
call 201 POST /api/v1/admin/articles "scheduled; sanitised" "${A[@]}" -d "$ART"
ART_ID=$(jqo .data.id); ART_SLUG=$(jqo .data.slug)
call 404 GET /api/v1/articles/$ART_SLUG "scheduled hidden publicly"
call 200 GET "/api/v1/admin/articles?status=scheduled" "" -b "$JAR"
call 200 GET /api/v1/admin/articles/$ART_ID "" -b "$JAR"
call 200 PUT /api/v1/admin/articles/$ART_ID "publish now (publishedAt null)" "${A[@]}" -d "$(echo "$ART" | jq -c '.publishedAt=null')"
call 200 GET /api/v1/articles/$ART_SLUG "now visible"
call 409 POST /api/v1/admin/articles "explicit slug taken" "${A[@]}" -d "$(echo "$ART" | jq -c --arg s "$ART_SLUG" '.slug=$s')"
call 413 POST /api/v1/admin/articles "body > 1 MiB" "${A[@]}" --data-binary @<(python3 -c "print('{\"title\":\"'+'a'*1100000+'\"}')")
# --- promos
NOW=$(date -u +%s)
iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }
P() { echo "{\"title\":\"$1\",\"serviceId\":$4,\"description\":\"\",\"code\":null,\"startsAt\":\"$(iso $2)\",\"endsAt\":\"$(iso $3)\"}"; }
call 201 POST /api/v1/admin/promos "active, draft service" "${A[@]}" -d "$(P Aktif $((NOW-86400)) $((NOW+86400*5)) "\"$SVC_ID\"")"
PR1=$(jqo .data.id)
call 201 POST /api/v1/admin/promos "ended 3 days ago" "${A[@]}" -d "$(P Lalu $((NOW-86400*10)) $((NOW-86400*3)) null)"
PR2=$(jqo .data.id)
call 201 POST /api/v1/admin/promos "ended 10 days ago" "${A[@]}" -d "$(P Lama $((NOW-86400*20)) $((NOW-86400*10)) null)"
PR3=$(jqo .data.id)
call 200 GET /api/v1/promos "active+ended≤7d; service null (draft)"
PUBLIC_PROMOS=$(jq -c '[.data[] | {title,status,service}]' "$OUT")
call 200 GET "/api/v1/promos?status=ended" ""
call 200 GET "/api/v1/admin/promos?pageSize=50" "admin sees all" -b "$JAR"
call 200 GET /api/v1/admin/promos/$PR1 "service shown in admin" -b "$JAR"
call 200 PUT /api/v1/admin/promos/$PR1 "" "${A[@]}" -d "$(P Aktif2 $((NOW-86400)) $((NOW+86400*5)) null)"
call 422 POST /api/v1/admin/promos "endsAt <= startsAt" "${A[@]}" -d "$(P Salah $NOW $NOW null)"
# --- testimonials & logos
call 201 POST /api/v1/admin/testimonials "" "${A[@]}" -d '{"clientName":"Budi","clientRole":"HR Manager","company":"PT Uji","quote":"Rapi dan terdokumentasi.","isActive":true}'
T1=$(jqo .data.id)
call 201 POST /api/v1/admin/testimonials "inactive" "${A[@]}" -d '{"clientName":"Ani","clientRole":"","company":"","quote":"Bagus.","isActive":false}'
T2=$(jqo .data.id)
call 200 GET /api/v1/admin/testimonials "" -b "$JAR"
call 200 GET /api/v1/admin/testimonials/$T1 "" -b "$JAR"
call 200 PUT /api/v1/admin/testimonials/$T2 "" "${A[@]}" -d '{"clientName":"Ani","clientRole":"CEO","company":null,"quote":"Bagus sekali.","isActive":false}'
call 200 PUT /api/v1/admin/testimonials/order "" "${A[@]}" -d "{\"ids\":[\"$T2\",\"$T1\"]}"
call 200 GET /api/v1/testimonials "only active"
# --- media
call 201 POST /api/v1/admin/media "png 120x40" -b "$JAR" -H "$CS" -F "file=@$F/logo.png"
LOGO_URL=$(jqo .data.url)
call 200 GET "$LOGO_URL" "served, nosniff"
call 415 POST /api/v1/admin/media "svg rejected" -b "$JAR" -H "$CS" -F "file=@$F/evil.svg;type=image/png"
call 413 POST /api/v1/admin/media "> 2 MiB" -b "$JAR" -H "$CS" -F "file=@$F/big.png"
call 201 POST /api/v1/admin/client-logos "" "${A[@]}" -d "{\"name\":\"PT Uji\",\"imageUrl\":\"$LOGO_URL\"}"
L1=$(jqo .data.id)
call 201 POST /api/v1/admin/client-logos "" "${A[@]}" -d '{"name":"PT Dua","imageUrl":"https://cdn.example.com/a.webp"}'
L2=$(jqo .data.id)
call 422 POST /api/v1/admin/client-logos "javascript: URL" "${A[@]}" -d '{"name":"X","imageUrl":"javascript:alert(1)"}'
call 200 GET /api/v1/admin/client-logos "" -b "$JAR"
call 200 GET /api/v1/admin/client-logos/$L1 "" -b "$JAR"
call 200 PUT /api/v1/admin/client-logos/$L1 "" "${A[@]}" -d "{\"name\":\"PT Uji 2\",\"imageUrl\":\"$LOGO_URL\"}"
call 200 PUT /api/v1/admin/client-logos/order "" "${A[@]}" -d "{\"ids\":[\"$L2\",\"$L1\"]}"
call 200 GET /api/v1/client-logos ""
# --- leads (admin)
call 200 GET "/api/v1/admin/leads?status=new&q=Sari" "" -b "$JAR"
call 200 GET /api/v1/admin/leads/$LEAD_ID "whatsapp +6281234567890" -b "$JAR"
LEAD_WA=$(jqo .data.whatsapp)
call 200 PATCH /api/v1/admin/leads/$LEAD_ID "status + notes" "${A[@]}" -d '{"status":"contacted","notes":"Sudah ditelepon."}'
call 422 PATCH /api/v1/admin/leads/$LEAD_ID "{} → 422" "${A[@]}" -d '{}'
call 200 GET "/api/v1/admin/leads/export" "CSV with BOM" -b "$JAR"
CSV_HEAD=$(head -c 120 "$OUT" | od -c | head -2 | tr -s ' ' | cut -c1-80)
# --- users
call 200 GET /api/v1/admin/users "" -b "$JAR"
call 201 POST /api/v1/admin/users "editor" "${A[@]}" -d '{"name":"Editor Smoke","email":"editor@mindtoscreen.com","role":"editor","password":"Editor-Smoke-1"}'
U1=$(jqo .data.id)
call 409 POST /api/v1/admin/users "email taken (case-insensitive)" "${A[@]}" -d '{"name":"X","email":"EDITOR@mindtoscreen.com","role":"editor","password":"Editor-Smoke-1"}'
call 200 GET /api/v1/admin/users/$U1 "" -b "$JAR"
call 200 PUT /api/v1/admin/users/$U1 "" "${A[@]}" -d '{"name":"Editor Smoke 2","email":"editor@mindtoscreen.com","role":"editor","isActive":true}'
call 204 POST /api/v1/admin/users/$U1/password "" "${A[@]}" -d '{"password":"Editor-Smoke-2"}'
ME=$(curl -s -b "$JAR" "$B/api/v1/auth/me" | jq -r .data.id)
call 409 PUT /api/v1/admin/users/$ME "self role change" "${A[@]}" -d "{\"name\":\"Admin\",\"email\":\"$ADMIN_EMAIL\",\"role\":\"editor\",\"isActive\":true}"
call 409 DELETE /api/v1/admin/users/$ME "self delete" -b "$JAR" -H "$CS"
# --- editor role gating
call 200 POST /api/v1/auth/login "editor login" -H "$J" -H "$CS" -c "$EJAR" -d '{"email":"editor@mindtoscreen.com","password":"Editor-Smoke-2"}'
E=(-b "$EJAR" -H "$CS" -H "$J")
call 200 GET /api/v1/admin/settings "editor may read" -b "$EJAR"
call 403 PUT /api/v1/admin/settings "editor forbidden" "${E[@]}" -d "$SET"
call 200 PATCH /api/v1/admin/settings/toggles "editor allowed" "${E[@]}" -d '{"showTestimonials":true}'
call 403 GET /api/v1/admin/users "editor forbidden" -b "$EJAR"
call 200 GET /api/v1/admin/leads "editor may read" -b "$EJAR"
call 403 PATCH /api/v1/admin/leads/$LEAD_ID "editor forbidden" "${E[@]}" -d '{"status":"done"}'
call 403 GET /api/v1/admin/leads/export "editor forbidden" -b "$EJAR"
call 200 GET /api/v1/admin/dashboard "editor allowed" -b "$EJAR"
call 204 POST /api/v1/auth/logout "editor logout" -b "$EJAR" -c "$EJAR" -H "$CS"
# --- deletes
call 204 DELETE /api/v1/admin/users/$U1 "" -b "$JAR" -H "$CS"
call 204 DELETE /api/v1/admin/client-logos/$L1 "" -b "$JAR" -H "$CS"
call 204 DELETE /api/v1/admin/client-logos/$L2 "" -b "$JAR" -H "$CS"
call 204 DELETE /api/v1/admin/testimonials/$T1 "" -b "$JAR" -H "$CS"
call 204 DELETE /api/v1/admin/testimonials/$T2 "" -b "$JAR" -H "$CS"
for p in $PR1 $PR2 $PR3; do call 204 DELETE /api/v1/admin/promos/$p "" -b "$JAR" -H "$CS"; done
call 204 DELETE /api/v1/admin/articles/$ART_ID "" -b "$JAR" -H "$CS"
call 204 DELETE /api/v1/admin/case-studies/$CS_ID "" -b "$JAR" -H "$CS"
call 204 DELETE /api/v1/admin/services/$SVC_ID "" -b "$JAR" -H "$CS"
call 404 DELETE /api/v1/admin/services/$SVC_ID "already gone" -b "$JAR" -H "$CS"
call 204 POST /api/v1/auth/logout "admin logout" -b "$JAR" -c "$JAR" -H "$CS"
call 401 GET /api/v1/auth/me "after logout" -b "$JAR"
echo
echo "PASS=$PASS FAIL=$FAILN"
rm -rf "$F" "$JAR" "$EJAR" "$OUT"
echo "LEAD_WA=$LEAD_WA"
echo "PUBLIC_PROMOS=$PUBLIC_PROMOS"
echo "CSV_HEAD=$CSV_HEAD"
[ "$FAILN" -eq 0 ]
