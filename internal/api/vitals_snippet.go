package api

import "net/http"

// vitalsSnippetJS has no dependencies; thresholds live server side so the script stays tiny.
const vitalsSnippetJS = `(function(){var s=document.currentScript,d=s.dataset,m={},c=0;
if(!d.app||!d.key||Math.random()*100>=(+d.sample||100))return;
var u=new URL(s.src).origin+"/api/v1/vitals/"+encodeURIComponent(d.app);
var ob=[],sent=0;
function o(t,f,x){try{var p=new PerformanceObserver(function(l){l.getEntries().forEach(f)});p.observe(Object.assign({type:t,buffered:true},x));ob.push([p,f])}catch(e){}}
o("largest-contentful-paint",function(e){m.LCP=e.startTime});
o("layout-shift",function(e){if(!e.hadRecentInput){c+=e.value;m.CLS=c}});
o("event",function(e){if(e.interactionId&&e.duration>(m.INP||0))m.INP=e.duration},{durationThreshold:40});
o("paint",function(e){if(e.name==="first-contentful-paint")m.FCP=e.startTime});
var n=performance.getEntriesByType("navigation")[0];if(n)m.TTFB=n.responseStart;
function send(){if(sent)return;sent=1;ob.forEach(function(r){r[0].takeRecords().forEach(r[1]);r[0].disconnect()});var l=Object.keys(m).map(function(k){return{n:k,v:m[k]}});if(l.length)navigator.sendBeacon(u,JSON.stringify({key:d.key,path:location.pathname,m:l}))}
addEventListener("visibilitychange",function(){if(document.visibilityState==="hidden")send()});
addEventListener("pagehide",send);
})();
`

// handleVitalsSnippet serves the browser script, public and cacheable like any static asset.
func (rt *Router) handleVitalsSnippet(w http.ResponseWriter, _ *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(vitalsSnippetJS))
}
