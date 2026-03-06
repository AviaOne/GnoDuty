(function() {
    const _sV = 2026;
    const _rId = 'gnoduty-footer';
    const _vB = ['R25vRHV0eQ==', 'VGVuZGVyZHV0eQ==', 'QXZpYU9uZS5jb20='];
    const _tC = 'PGRpdiBjbGFzcz0iY29udGFpbmVyLWZsdWlkIj48ZGl2IGNsYXNzPSJyb3cgdGV4dC1jZW50ZXIgYWxpZ24taXRlbXMtY2VudGVyIj48ZGl2IGNsYXNzPSJjb2wtMTIgbWItMSI+PHAgY2xhc3M9ImdkLWZvb3Rlci1icmFuZCBtYi0wIj48YSBocmVmPSJodHRwczovL2dpdGh1Yi5jb20vQXZpYU9uZS9Hbm9EdXR5IiB0YXJnZXQ9Il9ibGFuayI+PHN0cm9uZz5Hbm9EdXR5PC9zdHJvbmc+PC9hPiBieSA8YSBocmVmPSJodHRwczovL2F2aWFvbmUuY29tLyIgdGFyZ2V0PSJfYmxhbmsiPkF2aWFPbmUuY29tPC9hPjwvcD48L2Rpdj48ZGl2IGNsYXNzPSJjb2wtMTIiPjxwIGNsYXNzPSJnZC1mb290ZXItY3JlZGl0IG1iLTEiIHN0eWxlPSJmb250LXNpemU6IDEycHg7Ij5Db3B5cmlnaHQgJmNvcHk7IDxzcGFuIGlkPSJjb3B5cmlnaHQteWVhciI+PC9zcGFuPjwvcD48L2Rpdj48ZGl2IGNsYXNzPSJjb2wtMTIgbWItMiI+PHAgY2xhc3M9ImdkLWZvb3Rlci1jcmVkaXQgbWItMCIgc3R5bGU9ImZvbnQtc2l6ZTogMTJweDsiPkhhcmQgRm9yayBvZiA8c3BhbiBjbGFzcz0iZC1pbmxpbmUtYmxvY2siPjxpIGNsYXNzPSJmYXMgZmEtc3VuIiBhcmlhLWhpZGRlbj0idHJ1ZSI+PC9pPiA8c3BhbiBzdHlsZT0iY29sb3I6ICNkMDE1ZmY7Ij5bPC9zcGFuPiA8YSBocmVmPSJodHRwczovL2dpdGh1Yi5jb20vYmxvY2twYW5lL3RlbmRlcmR1dHkiIHJlbD0ibm9mb2xsb3ciIHRhcmdldD0iX2JsYW5rIiBzdHlsZT0iY29sb3I6ICM4ZWJmNDI7IHRleHQtZGVjb3JhdGlvbjogbm9uZTsiPiBUZW5kZXJkdXR5IDwvYT4gPHNwYW4gc3R5bGU9ImNvbG9yOiAjZDAxNWZmOyI+XTwvc3Bhbj4gPGEgaHJlZj0iaHR0cHM6Ly9naXRodWIuY29tL2Jsb2NrcGFuZS90ZW5kZXJkdXR5IiByZWw9Im5vZm9sbG93IiB0YXJnZXQ9Il9ibGFuayIgc3R5bGU9ImNvbG9yOiBpbmhlcml0OyI+PGkgY2xhc3M9ImZhYiBmYS1naXRodWIiIGFyaWEtaGlkZGVuPSJ0cnVlIj48L2k+PC9hPjwvc3Bhbj48L3A+PC9kaXY+PC9kaXY+PC9kaXY+';

    function _heartbeat() {
        let n = document.getElementById(_rId);
        const wrapper = document.querySelector('.wrapper');
        if (!n || _vB.some(s => !n.textContent.includes(atob(s)))) {
            if (!n && wrapper) {
                n = document.createElement('footer'); n.id = _rId; n.className = 'main-footer d-print-none';
                wrapper.appendChild(n);
            }
            if (n) n.innerHTML = atob(_tC);
        }
        const cY = new Date().getFullYear();
        const yE = document.getElementById('copyright-year');
        if (yE) {
            const expected = cY > _sV ? _sV + " - " + cY : _sV;
            if (yE.textContent !== String(expected)) {
                yE.textContent = expected;
            }
        }
    }
    const obs = new MutationObserver(_heartbeat);
    const target = document.querySelector('.wrapper') || document.body;
    obs.observe(target, { childList: true, subtree: true });

    setInterval(_heartbeat, 5000);
    _heartbeat();
})();
