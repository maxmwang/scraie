-- Stores inputs to SerpAPI (https://serpapi.com/google-flights-api)
CREATE TABLE IF NOT EXISTS Itineraries (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    departure_id     TEXT    NOT NULL,
    arrival_id       TEXT    NOT NULL,
    type             INTEGER NOT NULL DEFAULT 1,
    outbound_date    TEXT    NOT NULL,
    return_date      TEXT,
    travel_class     INTEGER NOT NULL DEFAULT 1,
    show_hidden      BOOLEAN NOT NULL DEFAULT TRUE, -- SerpAPI default is FALSE.
    multi_city_json  TEXT,
    exclude_basic    BOOLEAN NOT NULL DEFAULT FALSE,
    deep_search      BOOLEAN NOT NULL DEFAULT TRUE, -- SerpAPI default is FALSE.
    adults           INTEGER NOT NULL DEFAULT 1,
    children         INTEGER NOT NULL DEFAULT 0,
    infants_in_seat  INTEGER NOT NULL DEFAULT 0,
    infants_on_lap   INTEGER NOT NULL DEFAULT 0,
    sort_by          INTEGER NOT NULL DEFAULT 1,
    stops            INTEGER NOT NULL DEFAULT 0,
    exclude_airlines TEXT,
    include_airlines TEXT,
    bags             INTEGER NOT NULL DEFAULT 0,
    max_price        INTEGER,
    outbound_times   TEXT,
    return_times     TEXT,
    emissions        INTEGER,
    layover_duration TEXT,
    exclude_conns    TEXT,
    max_duration     INTEGER,
    gl               TEXT    NOT NULL DEFAULT 'us',
    hl               TEXT    NOT NULL DEFAULT 'en',
    currency         TEXT    NOT NULL DEFAULT 'USD',

    invalid          BOOLEAN NOT NULL DEFAULT FALSE,
    notify           BOOLEAN NOT NULL DEFAULT FALSE,
    description      TEXT
);

CREATE TABLE IF NOT EXISTS Options (
    id               BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    itinerary_id     BIGINT      NOT NULL REFERENCES Itineraries(id),

    total_duration   INTEGER     NOT NULL,
    price            INTEGER     NOT NULL,
    type             TEXT        NOT NULL,
    searched_at      TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS Segments (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    option_id              BIGINT  NOT NULL REFERENCES Options(id),

    departure_airport_name TEXT    NOT NULL,
    departure_airport_id   TEXT    NOT NULL,
    departure_time         TEXT    NOT NULL,
    arrival_airport_name   TEXT    NOT NULL,
    arrival_airport_id     TEXT    NOT NULL,
    arrival_time           TEXT    NOT NULL,

    duration               INTEGER NOT NULL,
    airplane               TEXT    NOT NULL,
    airline                TEXT    NOT NULL,
    airline_logo           TEXT    NOT NULL DEFAULT '',
    travel_class           TEXT    NOT NULL,
    flight_number          TEXT    NOT NULL,
    overnight              BOOLEAN NOT NULL
);

CREATE TABLE IF NOT EXISTS Layover (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    option_id BIGINT  NOT NULL REFERENCES Options(id),

    duration    INTEGER NOT NULL,
    name        TEXT    NOT NULL,
    airport_id  TEXT    NOT NULL,
    overnight   BOOLEAN NOT NULL
);
