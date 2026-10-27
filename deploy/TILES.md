# Тайлы карты

Карта на сайте берёт тайлы через кеширующий прокси сервера (`/tiles/…`):
каждый тайл запрашивается у источника один раз, потом отдаётся из кеша
неделю. Источник задаёт `TILE_URL` в `.env`.

## Когда что

| Посещаемость | Источник | Что сделать |
|---|---|---|
| Десятки–сотни человек в день | tile.openstreetmap.org (по умолчанию) | ничего; правила OSM разрешают небольшой трафик при кеше и честном User-Agent — у нас так и есть |
| Тысячи в день, без админки карты | платный сервис | ключ в `TILE_URL`, см. ниже |
| Тысячи в день, хочется своего | свой тайл-сервер (профиль `tiles`) | импорт области, ~2 часа один раз |

Правила tile.openstreetmap.org: https://operations.osmfoundation.org/policies/tiles/ —
при росте трафика OSM может ограничить доступ, лучше перейти заранее.

## Платный сервис

Подходят любые растровые тайлы по шаблону `{z}/{x}/{y}`. Для походов удобны
карты с горизонталями и тропами (MapTiler Outdoor, Thunderforest Outdoors,
Stadia Outdoors). Пример:

```
TILE_URL=https://api.maptiler.com/maps/outdoor-v2/256/{z}/{x}/{y}.png?key=ВАШ_КЛЮЧ
TILE_ATTRIBUTION=© MapTiler © участники OpenStreetMap
TILE_MAX_ZOOM=18
```

Ключ остаётся на сервере: браузер видит только `/tiles/…`.

## Свой тайл-сервер

Образ `overv/openstreetmap-tile-server` рисует тайлы в стиле
openstreetmap.org из выгрузки OSM. Нужно: 4 vCPU, 8 ГБ памяти и ~40 ГБ
диска на импорт Южного Урала (база ~15 ГБ, остальное — временное). Лучше
отдельный сервер; на основном — только если ресурсов с запасом.

1. **Выгрузка области.** Южный Урал лежит в двух округах Geofabrik
   (Челябинская обл. — Уральский, Башкирия с Иремелем — Приволжский).
   Скачать оба и вырезать прямоугольник:

   ```bash
   mkdir -p /opt/osm && cd /opt/osm
   wget https://download.geofabrik.de/russia/ural-fed-district-latest.osm.pbf
   wget https://download.geofabrik.de/russia/volga-fed-district-latest.osm.pbf
   docker run --rm -v "$PWD:/w" -w /w debian:12 sh -c \
     'apt-get update -qq && apt-get install -y -qq osmium-tool >/dev/null &&
      osmium merge ural-fed-district-latest.osm.pbf volga-fed-district-latest.osm.pbf -o merged.osm.pbf &&
      osmium extract -b 56.5,52.5,62.5,56.6 merged.osm.pbf -o south-ural.osm.pbf'
   ```

   Прямоугольник `-b долгота_мин,широта_мин,долгота_макс,широта_макс`
   покрывает Таганай, Зюраткуль, Иремель, Ямантау и окрестности
   Челябинска, Миасса, Уфы. Поменяйте под себя.

2. **Импорт** (1–3 часа):

   ```bash
   cd /opt/socialnet
   docker compose --profile tiles run --rm \
     -v /opt/osm/south-ural.osm.pbf:/data/region.osm.pbf:ro tiles import
   ```

3. **Запуск** и переключение карты:

   ```bash
   docker compose --profile tiles up -d tiles
   ```

   В `.env`:

   ```
   TILE_URL=http://tiles/tile/{z}/{x}/{y}.png
   TILE_MAX_ZOOM=18
   ```

   и `deploy/update.sh` (или `docker compose up -d api`).

4. **Проверка:** открыть маршрут на карте. Первые показы медленные — тайлы
   рисуются по запросу и складываются в том `tilescache`; дальше их отдаёт
   и наш прокси, и сам тайл-сервер.

Данные не обновляются сами: раз в несколько месяцев повторите шаги 1–2
(старую базу — `docker volume rm socialnet_tilesdb` после остановки `tiles`).
Атрибуция «© участники OpenStreetMap» обязательна (лицензия ODbL) — она
стоит по умолчанию.
