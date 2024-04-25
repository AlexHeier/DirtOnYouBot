CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS public.messages (
    userid character varying(255) COLLATE pg_catalog."default",
    timestamp timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    message text COLLATE pg_catalog."default",
    serverid character varying(255) COLLATE pg_catalog."default",
    wordid uuid[]
);


CREATE TABLE IF NOT EXISTS public.words (
    wordid uuid NOT NULL DEFAULT uuid_generate_v4(),
    word character varying(255) COLLATE pg_catalog."default",
    CONSTRAINT words_pkey PRIMARY KEY (wordid),
    CONSTRAINT words_word_key UNIQUE (word)
);

ALTER TABLE IF EXISTS public.messages OWNER TO postgres;
ALTER TABLE IF EXISTS public.words OWNER TO postgres;
