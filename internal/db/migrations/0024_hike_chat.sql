-- Чат похода: создаётся сам, участники — организатор и все, кто идёт.
-- Состав ведёт сервер по списку группы; удалили поход — чат остаётся
-- обычной беседой с историей.
ALTER TABLE conversations
    ADD COLUMN event_id BIGINT UNIQUE REFERENCES events(id) ON DELETE SET NULL;
