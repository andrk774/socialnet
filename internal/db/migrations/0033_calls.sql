CREATE TABLE calls (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 caller_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 callee_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 video boolean NOT NULL DEFAULT false,
 status text NOT NULL DEFAULT 'ringing' CHECK(status IN ('ringing','active','ended','declined','missed')),
 offer text NOT NULL DEFAULT '',
 answer text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 answered_at timestamptz,
 ended_at timestamptz,
 caller_seen timestamptz NOT NULL DEFAULT now(),
 callee_seen timestamptz NOT NULL DEFAULT now(),
 CHECK(caller_id<>callee_id)
);
CREATE INDEX calls_caller_idx ON calls(caller_id,id DESC);
CREATE INDEX calls_callee_idx ON calls(callee_id,id DESC);
CREATE TABLE call_history_hidden (
 call_id bigint NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 PRIMARY KEY(call_id,user_id)
);
