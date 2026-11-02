package configtranslate

import "fmt"

// cronCallScript calls the app over loopback and sends Vercel's bearer header
// when CRON_SECRET is set in the container. It reads the variable at run time,
// so the secret value never appears in the job definition. It tries curl, then
// wget, then node, because slim and alpine images often lack curl.
const cronCallScript = `u="$1"
if command -v curl >/dev/null 2>&1; then
  if [ -n "$CRON_SECRET" ]; then exec curl -fsS -H "Authorization: Bearer $CRON_SECRET" "$u"; fi
  exec curl -fsS "$u"
fi
if command -v wget >/dev/null 2>&1; then
  if [ -n "$CRON_SECRET" ]; then exec wget -q -O- --header="Authorization: Bearer $CRON_SECRET" "$u"; fi
  exec wget -q -O- "$u"
fi
if command -v node >/dev/null 2>&1; then
  exec node -e 'const h={};if(process.env.CRON_SECRET)h.authorization="Bearer "+process.env.CRON_SECRET;fetch(process.argv[1],{headers:h}).then(r=>process.exit(r.ok?0:1),()=>process.exit(1))' "$u"
fi
echo "cron call needs curl, wget or node in the image" >&2
exit 127`

// cronCommand is the exec argv for one translated Vercel cron.
func cronCommand(port int, path string) []string {
	return []string{"sh", "-c", cronCallScript, "cron", cronURL(port, path)}
}

func cronURL(port int, path string) string { return fmt.Sprintf("http://127.0.0.1:%d%s", port, path) }
