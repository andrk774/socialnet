import { h } from '../vendor/preact/index.js';
import { useRef, useState } from '../vendor/preact/hooks.js';
import { get, put, upload } from './api.js';
import { useLoad, ErrorLine } from './ui.js';
import { Icon } from './shellparts.js';
export function ProfileCover({ id, self }) {
    const cover = useLoad(() => get(`/users/${id}/cover`), [id]);
    const input = useRef(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState(null);
    async function save(file) { setBusy(true); setError(null); try {
        let media_id = null;
        if (file) {
            if (!['image/jpeg', 'image/png', 'image/gif'].includes(file.type) || file.size > 10 * 1024 * 1024)
                throw new Error('Выберите JPEG, PNG или GIF до 10 МБ.');
            media_id = (await upload('/media', file)).id;
        }
        await put('/users/me/cover', { media_id });
        cover.reload();
    }
    catch (e) {
        setError(e);
    }
    finally {
        setBusy(false);
        if (input.current)
            input.current.value = '';
    } }
    return h("div", { class: "profile-cover" },
        cover.data?.url && h("img", { src: cover.data.url, alt: "\u041E\u0431\u043B\u043E\u0436\u043A\u0430 \u043F\u0440\u043E\u0444\u0438\u043B\u044F" }),
        self && h("details", { class: "cover-menu" },
            h("summary", null,
                h(Icon, { name: "edit" }),
                " \u0418\u0437\u043C\u0435\u043D\u0438\u0442\u044C \u043E\u0431\u043B\u043E\u0436\u043A\u0443"),
            h("div", null,
                h("button", { disabled: busy, onClick: () => input.current?.click() },
                    h(Icon, { name: "photo" }),
                    "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044C \u0438\u0437\u043E\u0431\u0440\u0430\u0436\u0435\u043D\u0438\u0435"),
                h("button", { class: "danger", disabled: busy || !cover.data?.url, onClick: () => { if (confirm('Удалить обложку профиля?'))
                        void save(); } },
                    h(Icon, { name: "trash" }),
                    "\u0423\u0434\u0430\u043B\u0438\u0442\u044C"))),
        h("input", { hidden: true, ref: input, type: "file", accept: "image/jpeg,image/png,image/gif", onChange: e => { const f = e.currentTarget.files?.[0]; if (f)
                void save(f); } }),
        h(ErrorLine, { error: error || cover.error }));
}
