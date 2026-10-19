package edgefw

import (
	"crypto/rand"
	"encoding/base64"
	"html"
	"strconv"
	"strings"
)

const challengeScript = `(function(){
var K=[],H0=[];
(function(){var n=2,c=0;while(c<64){var p=1;for(var d=2;d*d<=n;d++){if(n%d===0){p=0;break}}
if(p){if(c<8)H0[c]=(Math.pow(n,.5)%1)*4294967296|0;K[c]=(Math.pow(n,1/3)%1)*4294967296|0;c++}n++}})();
function sha(msg){
var l=msg.length,n=((l+8)>>6)+1,m=new Uint32Array(n*16),w=new Uint32Array(64),H=H0.slice(),i,b;
for(i=0;i<l;i++)m[i>>2]|=msg.charCodeAt(i)<<(24-(i&3)*8);
m[l>>2]|=0x80<<(24-(l&3)*8);m[n*16-1]=l*8;
for(b=0;b<n;b++){
for(i=0;i<16;i++)w[i]=m[b*16+i];
for(i=16;i<64;i++){var a=w[i-15],c=w[i-2];
w[i]=(w[i-16]+(((a>>>7)|(a<<25))^((a>>>18)|(a<<14))^(a>>>3))+w[i-7]+(((c>>>17)|(c<<15))^((c>>>19)|(c<<13))^(c>>>10)))|0}
var A=H[0],B=H[1],C=H[2],D=H[3],E=H[4],F=H[5],G=H[6],X=H[7];
for(i=0;i<64;i++){
var t1=(X+(((E>>>6)|(E<<26))^((E>>>11)|(E<<21))^((E>>>25)|(E<<7)))+((E&F)^(~E&G))+K[i]+w[i])|0;
var t2=((((A>>>2)|(A<<30))^((A>>>13)|(A<<19))^((A>>>22)|(A<<10)))+((A&B)^(A&C)^(B&C)))|0;
X=G;G=F;F=E;E=(D+t1)|0;D=C;C=B;B=A;A=(t1+t2)|0}
H[0]=(H[0]+A)|0;H[1]=(H[1]+B)|0;H[2]=(H[2]+C)|0;H[3]=(H[3]+D)|0;H[4]=(H[4]+E)|0;H[5]=(H[5]+F)|0;H[6]=(H[6]+G)|0;H[7]=(H[7]+X)|0}
return H}
function zeros(h){return h[0]!==0?Math.clz32(h[0]):32+Math.clz32(h[1])}
var t=document.getElementById('lr-token').value,need=parseInt(document.getElementById('lr-bits').value,10);
var status=document.getElementById('lr-status'),bar=document.getElementById('lr-bar'),retry=document.getElementById('lr-retry');
function fail(){bar.hidden=true;status.textContent=retry.getAttribute('data-failed');retry.hidden=false}
retry.addEventListener('click',function(){location.reload()});
var n=0;
function work(){
var end=Date.now()+25;
while(Date.now()<end){if(zeros(sha(t+':'+n))>=need){return done(n)}n++}
setTimeout(work,0)}
function done(c){
var body='t='+encodeURIComponent(t)+'&n='+c;
fetch(location.origin+'` + ChallengePath + `',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:body})
.then(function(r){if(r.ok){location.replace(location.href)}else{fail()}}).catch(fail)}
work()})();`

const challengeTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex">
<title>{{TITLE}}</title>
<style nonce="{{NONCE}}">
:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 system-ui,sans-serif;background:Canvas;color:CanvasText}
main{max-width:28rem;padding:1.5rem;text-align:center}
h1{font-size:1.25rem;margin:0 0 .5rem}
p{margin:.5rem 0;opacity:.8}
progress{width:100%;margin-top:1rem}
button{font:inherit;padding:.5rem 1rem;margin-top:1rem;cursor:pointer}
button:focus-visible{outline:2px solid Highlight;outline-offset:2px}
</style>
</head>
<body>
<main>
<h1>Checking your browser</h1>
<p id="lr-status" role="status" aria-live="polite">This takes a moment. Nothing is sent to anyone else.</p>
<progress id="lr-bar" aria-label="Checking your browser"></progress>
<button id="lr-retry" type="button" hidden data-failed="The check did not finish. Try again.">Try again</button>
<noscript><p>JavaScript is needed to finish this check.</p></noscript>
<input type="hidden" id="lr-token" value="{{TOKEN}}">
<input type="hidden" id="lr-bits" value="{{BITS}}">
</main>
<script nonce="{{NONCE}}">{{SCRIPT}}</script>
</body>
</html>
`

// ChallengePage renders the self-contained proof-of-work page and the
// Content-Security-Policy header value that allows only its own script.
func ChallengePage(title, token string, difficulty int) (page []byte, csp string) {
	nb := make([]byte, 12)
	_, _ = rand.Read(nb)
	nonce := base64.RawURLEncoding.EncodeToString(nb)
	r := strings.NewReplacer(
		"{{TITLE}}", html.EscapeString(title),
		"{{NONCE}}", nonce,
		"{{TOKEN}}", html.EscapeString(token),
		"{{BITS}}", strconv.Itoa(difficulty),
		"{{SCRIPT}}", challengeScript,
	)
	csp = "default-src 'none'; script-src 'nonce-" + nonce + "'; style-src 'nonce-" + nonce + "'; connect-src 'self'; base-uri 'none'; form-action 'none'"
	return []byte(r.Replace(challengeTemplate)), csp
}
