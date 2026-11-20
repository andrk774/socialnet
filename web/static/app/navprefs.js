export const optionalSections = [["/feeds", "Мои ленты"], ["/communities", "Сообщества"], ["/hikes", "Походы"], ["/record", "Записать трек"], ["/companions", "Попутчики"], ["/rides", "Подвоз"], ["/articles", "Статьи"], ["/fundraisers", "Сборы"], ["/offline", "Офлайн"], ["/photos", "Фото"], ["/music", "Музыка"], ["/videos", "Видео"], ["/bookmarks", "Закладки"], ["/documents", "Документы"], ["/scheduled", "Отложенные"], ["/wallet", "Кошелёк"], ["/plus", "Плюс"]];
export function hiddenSections(userId) { try {
    const v = JSON.parse(localStorage.getItem(`sn-navigation-${userId}`) ?? "[]");
    return Array.isArray(v) ? v.filter(x => optionalSections.some(([url]) => url === x)) : [];
}
catch {
    return [];
} }
