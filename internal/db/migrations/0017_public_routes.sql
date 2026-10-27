-- Открытый каталог маршрутов: владелец сам решает показать маршрут всем,
-- в том числе без входа и поисковикам (/trails).
ALTER TABLE routes
    ADD COLUMN public      BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN public_note TEXT    NOT NULL DEFAULT '' CHECK (char_length(public_note) <= 2000),
    ADD COLUMN public_at   TIMESTAMPTZ;
CREATE INDEX routes_public_idx ON routes (public_at DESC) WHERE public AND deleted_at IS NULL AND hidden_at IS NULL;
