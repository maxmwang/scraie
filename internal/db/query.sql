-- name: GetItineraries :many
SELECT * FROM itineraries;

-- name: GetItinerary :one
SELECT * FROM itineraries WHERE id = $1;

-- name: GetUserByDiscordID :one
SELECT id, discord_id FROM users WHERE discord_id = $1;

-- name: CreateDiscordUser :one
INSERT INTO users (id, discord_id)
VALUES ($1, $2)
RETURNING id;

-- name: GetUserByID :one
SELECT id, discord_id FROM users WHERE id = $1;

-- name: GetItinerariesByUser :many
SELECT * FROM itineraries WHERE user_id = $1;

-- name: CreateItinerary :one
INSERT INTO itineraries (
    user_id, departure_id, arrival_id, type, outbound_date, return_date, travel_class,
    show_hidden, multi_city_json, exclude_basic, deep_search,
    adults, children, infants_in_seat, infants_on_lap,
    sort_by, stops, exclude_airlines, include_airlines, bags, max_price,
    outbound_times, return_times, emissions, layover_duration, exclude_conns,
    max_duration, gl, hl, currency
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18, $19, $20, $21,
    $22, $23, $24, $25, $26, $27,
    $28, $29, $30
)
RETURNING id;

-- name: UpdateItinerary :one
UPDATE itineraries
SET
    departure_id     = $2,
    arrival_id       = $3,
    type             = $4,
    outbound_date    = $5,
    return_date      = $6,
    travel_class     = $7,
    show_hidden      = $8,
    multi_city_json  = $9,
    exclude_basic    = $10,
    deep_search      = $11,
    adults           = $12,
    children         = $13,
    infants_in_seat  = $14,
    infants_on_lap   = $15,
    sort_by          = $16,
    stops            = $17,
    exclude_airlines = $18,
    include_airlines = $19,
    bags             = $20,
    max_price        = $21,
    outbound_times   = $22,
    return_times     = $23,
    emissions        = $24,
    layover_duration = $25,
    exclude_conns    = $26,
    max_duration     = $27,
    gl               = $28,
    hl               = $29,
    currency         = $30,
    description      = $31
WHERE id = $1
RETURNING id;

-- name: InvalidateItinerary :exec
UPDATE itineraries SET invalid = TRUE WHERE id = $1;

-- name: GetOptionsSince :many
-- Returns every option saved for an itinerary at or after the given timestamp,
-- ordered oldest first. Used to chart the recent price history of a flight.
SELECT * FROM options
WHERE itinerary_id = $1 AND searched_at >= $2
ORDER BY searched_at;

-- name: GetSegmentsByOptionIDs :many
-- Ordered by id (i.e. insertion order) so a multi-segment option's legs always
-- come back in the order they were searched/saved, keeping flight signatures
-- stable across searches.
SELECT id, option_id, departure_airport_name, departure_airport_id, departure_time, arrival_airport_name, arrival_airport_id, arrival_time, duration, airplane, airline, airline_logo, travel_class, flight_number, overnight
FROM segments
WHERE option_id = ANY($1::bigint[])
ORDER BY id;

-- name: GetLayoversByOptionIDs :many
SELECT id, option_id, duration, name, airport_id, overnight
FROM layover
WHERE option_id = ANY($1::bigint[]);

-- name: CreateOption :one
INSERT INTO options (itinerary_id, total_duration, price, type, searched_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: BulkInsertOptions :copyfrom
INSERT INTO options (itinerary_id, total_duration, price, type, searched_at)
VALUES ($1, $2, $3, $4, $5);

-- name: BulkInsertSegments :copyfrom
INSERT INTO segments (option_id, departure_airport_name, departure_airport_id, departure_time, arrival_airport_name, arrival_airport_id, arrival_time, duration, airplane, airline, airline_logo, travel_class, flight_number, overnight)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: BulkInsertLayovers :copyfrom
INSERT INTO layover (option_id, duration, name, airport_id, overnight)
VALUES ($1, $2, $3, $4, $5);
