#!/usr/bin/env bash
set -euo pipefail

URL="${1:-https://happier.example.com}"
N=5

HOST=$(echo "$URL" | sed 's|https://||;s|http://||' | cut -d/ -f1)

# Resolve for display only — never fatal. Try dig, then getent, then python3;
# fall back to "unknown" so a missing tool doesn't abort the measurement.
resolve() {
    if command -v dig >/dev/null 2>&1; then
        dig +short "$1" 2>/dev/null | tail -1 && return
    fi
    if command -v getent >/dev/null 2>&1; then
        getent hosts "$1" 2>/dev/null | awk '{print $1}' | tail -1 && return
    fi
    if command -v python3 >/dev/null 2>&1; then
        python3 -c "import socket,sys; print(socket.gethostbyname(sys.argv[1]))" "$1" 2>/dev/null && return
    fi
    echo "unknown"
}
RESOLVED=$(resolve "$HOST")
[ -z "$RESOLVED" ] && RESOLVED="unknown"

echo "Measuring $URL ($N runs)..."
echo "Resolved: $HOST -> $RESOLVED"
echo "Warming up..."
for _ in 1 2 3; do
    curl -o /dev/null -s "$URL"
done
echo ""

declare -a dns connect tls ttfb total

for i in $(seq 1 $N); do
    read -r d c t s o < <(curl -o /dev/null -s \
        -w "%{time_namelookup} %{time_connect} %{time_appconnect} %{time_starttransfer} %{time_total}" \
        "$URL") || true
    dns+=($d) connect+=($c) tls+=($t) ttfb+=($s) total+=($o)
    printf "  run %d: dns=%.0fms connect=%.0fms tls=%.0fms ttfb=%.0fms total=%.0fms\n" \
        $i $(echo "$d*1000"|bc) $(echo "$c*1000"|bc) $(echo "$t*1000"|bc) $(echo "$s*1000"|bc) $(echo "$o*1000"|bc)
done

echo ""

stats() {
    local label=$1; shift
    local values=("$@")
    awk -v label="$label" -v n="${#values[@]}" '
    BEGIN {
        split("'"${values[*]}"'", a, " ")
        sum=0; for(i=1;i<=n;i++) sum+=a[i]*1000
        mean=sum/n
        sq=0; for(i=1;i<=n;i++) sq+=(a[i]*1000-mean)^2
        sd=sqrt(sq/n)
        min=a[1]*1000; max=a[1]*1000
        for(i=2;i<=n;i++) { if(a[i]*1000<min) min=a[i]*1000; if(a[i]*1000>max) max=a[i]*1000 }
        printf "%-10s  mean=%5.0fms  sd=%4.0fms  min=%5.0fms  max=%5.0fms\n", label, mean, sd, min, max
    }' /dev/null
}

stats "dns"     "${dns[@]}"
stats "connect" "${connect[@]}"
stats "tls"     "${tls[@]}"
stats "ttfb"    "${ttfb[@]}"
stats "total"   "${total[@]}"
