import { h } from '../vendor/preact/index.js';
import { useEffect, useState } from '../vendor/preact/hooks.js';
import { get, put } from './api.js';
import { ErrorLine } from './ui.js';
export function SiteSettingsPanel() {
    const [data, setData] = useState();
    const [error, setError] = useState(null);
    const [busy, setBusy] = useState(false);
    const [saved, setSaved] = useState(false);
    useEffect(() => { void get('/admin/site').then(setData).catch(setError); }, []);
    function change(k, v) { if (data)
        setData({ ...data, [k]: v }); setSaved(false); }
    function download() { if (!data)
        return; try {
        const u = new URL(data.public_url);
        if (u.protocol !== 'https:' || u.port || u.hostname === 'localhost' || /^[\d.]+$/.test(u.hostname) || u.hostname.includes(':'))
            throw new Error('Для Ingress нужен домен HTTPS без номера порта.');
        const manifest = { apiVersion: 'networking.k8s.io/v1', kind: 'Ingress', metadata: { name: 'socialnet', namespace: 'socialnet' }, spec: { ingressClassName: data.ingress_class, tls: [{ hosts: [u.hostname], secretName: data.tls_secret }], rules: [{ host: u.hostname, http: { paths: [{ path: '/', pathType: 'Prefix', backend: { service: { name: 'socialnet', port: { number: 80 } } } }] } }] } };
        const url = URL.createObjectURL(new Blob([JSON.stringify(manifest, null, 2)], { type: 'application/json' }));
        const a = document.createElement('a');
        a.href = url;
        a.download = 'socialnet-ingress.json';
        a.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
    catch (e) {
        setError(e);
    } }
    return h("section", { class: "music-panel" },
        h("h2", null, "\u0410\u0434\u0440\u0435\u0441 \u0441\u0430\u0439\u0442\u0430 \u0438 HTTPS"),
        h(ErrorLine, { error: error }),
        data && h("form", { onSubmit: async (e) => { e.preventDefault(); setBusy(true); setError(null); setSaved(false); try {
                await put('/admin/site', data);
                setData(await get('/admin/site'));
                setSaved(true);
            }
            catch (e) {
                setError(e);
            }
            finally {
                setBusy(false);
            } } },
            h("fieldset", { disabled: busy, style: "display:grid;gap:14px;border:0;padding:0;min-width:0" },
                h("label", null,
                    "\u0410\u0434\u0440\u0435\u0441 socialnet",
                    h("input", { type: "url", required: true, maxLength: 512, placeholder: "https://socialnet.example.com", value: data.public_url, onInput: e => change('public_url', e.currentTarget.value) })),
                h("p", { class: "muted" }, "\u042D\u0442\u043E\u0442 \u0430\u0434\u0440\u0435\u0441 \u0438\u0441\u043F\u043E\u043B\u044C\u0437\u0443\u0435\u0442\u0441\u044F \u0432 \u043D\u043E\u0432\u044B\u0445 QR-\u043A\u043E\u0434\u0430\u0445 \u0432\u0445\u043E\u0434\u0430 \u0438 \u043F\u0438\u0441\u044C\u043C\u0430\u0445 \u0430\u043A\u043A\u0430\u0443\u043D\u0442\u0430. \u0414\u043B\u044F \u0442\u0435\u043B\u0435\u0444\u043E\u043D\u0430 \u0443\u043A\u0430\u0436\u0438\u0442\u0435 \u0434\u043E\u0441\u0442\u0443\u043F\u043D\u044B\u0439 \u0435\u043C\u0443 \u0434\u043E\u043C\u0435\u043D \u0438\u043B\u0438 \u0430\u0434\u0440\u0435\u0441 \u043A\u043E\u043C\u043F\u044C\u044E\u0442\u0435\u0440\u0430 \u0432 Wi-Fi. localhost \u0440\u0430\u0431\u043E\u0442\u0430\u0435\u0442 \u0442\u043E\u043B\u044C\u043A\u043E \u043D\u0430 \u0442\u0435\u043A\u0443\u0449\u0435\u043C \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0435."),
                h("label", null,
                    "\u041A\u043B\u0430\u0441\u0441 Ingress \u0432 Kubernetes",
                    h("input", { required: true, value: data.ingress_class, onInput: e => change('ingress_class', e.currentTarget.value) })),
                h("label", null,
                    "\u041D\u0430\u0437\u0432\u0430\u043D\u0438\u0435 Secret \u0441 TLS-\u0441\u0435\u0440\u0442\u0438\u0444\u0438\u043A\u0430\u0442\u043E\u043C",
                    h("input", { required: true, value: data.tls_secret, onInput: e => change('tls_secret', e.currentTarget.value) })),
                h("p", null,
                    "Secure-cookie \u0432 \u0442\u0435\u043A\u0443\u0449\u0435\u0439 \u043A\u043E\u043D\u0444\u0438\u0433\u0443\u0440\u0430\u0446\u0438\u0438: ",
                    data.cookie_secure ? 'включены' : 'выключены',
                    ". \u041C\u0435\u043D\u044F\u0435\u0442\u0441\u044F \u043F\u0430\u0440\u0430\u043C\u0435\u0442\u0440\u043E\u043C COOKIE_SECURE \u043F\u0440\u0438 \u0440\u0430\u0437\u0432\u0451\u0440\u0442\u044B\u0432\u0430\u043D\u0438\u0438."),
                h("p", { class: "muted" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u0438\u0435 \u0430\u0434\u0440\u0435\u0441\u0430 \u043D\u0435 \u0432\u043A\u043B\u044E\u0447\u0430\u0435\u0442 HTTPS \u0438 \u043D\u0435 \u0432\u044B\u043F\u0443\u0441\u043A\u0430\u0435\u0442 \u0441\u0435\u0440\u0442\u0438\u0444\u0438\u043A\u0430\u0442. \u0414\u043B\u044F \u0434\u043E\u043C\u0435\u043D\u0430 \u0443\u0441\u0442\u0430\u043D\u043E\u0432\u0438\u0442\u0435 Ingress-\u043A\u043E\u043D\u0442\u0440\u043E\u043B\u043B\u0435\u0440, \u043D\u0430\u043F\u0440\u0430\u0432\u044C\u0442\u0435 DNS \u043D\u0430 \u0442\u043E\u0447\u043A\u0443 \u0432\u0445\u043E\u0434\u0430 \u0438 \u0441\u043E\u0437\u0434\u0430\u0439\u0442\u0435 \u0443\u043A\u0430\u0437\u0430\u043D\u043D\u044B\u0439 TLS Secret \u0432 namespace socialnet. \u0414\u043B\u044F \u043B\u043E\u043A\u0430\u043B\u044C\u043D\u043E\u0433\u043E \u0441\u0435\u0440\u0442\u0438\u0444\u0438\u043A\u0430\u0442\u0430 \u043F\u043E\u0442\u0440\u0435\u0431\u0443\u0435\u0442\u0441\u044F \u0434\u043E\u0432\u0435\u0440\u0438\u0435 \u043D\u0430 \u043A\u0430\u0436\u0434\u043E\u043C \u0443\u0441\u0442\u0440\u043E\u0439\u0441\u0442\u0432\u0435."),
                h("div", { class: "row" },
                    h("button", { class: "primary" }, "\u0421\u043E\u0445\u0440\u0430\u043D\u0438\u0442\u044C \u0430\u0434\u0440\u0435\u0441 \u0438 \u043D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438"),
                    h("button", { type: "button", onClick: download }, "\u0421\u043A\u0430\u0447\u0430\u0442\u044C \u043A\u043E\u043D\u0444\u0438\u0433\u0443\u0440\u0430\u0446\u0438\u044E Ingress"))),
            saved && h("p", { role: "status" }, "\u041D\u0430\u0441\u0442\u0440\u043E\u0439\u043A\u0438 \u0441\u043E\u0445\u0440\u0430\u043D\u0435\u043D\u044B. \u041D\u043E\u0432\u044B\u0435 \u0441\u0441\u044B\u043B\u043A\u0438 \u0438\u0441\u043F\u043E\u043B\u044C\u0437\u0443\u044E\u0442 \u0443\u043A\u0430\u0437\u0430\u043D\u043D\u044B\u0439 \u0430\u0434\u0440\u0435\u0441. \u0414\u043E\u0441\u0442\u0443\u043F\u043D\u043E\u0441\u0442\u044C HTTPS \u0435\u0449\u0451 \u043D\u0435 \u043F\u0440\u043E\u0432\u0435\u0440\u0435\u043D\u0430.")));
}
