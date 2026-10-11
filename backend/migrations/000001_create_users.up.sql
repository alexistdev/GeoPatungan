/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */
CREATE TABLE users (
                       id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       name          TEXT NOT NULL,
                       phone         TEXT NOT NULL UNIQUE,
                       email         TEXT NOT NULL UNIQUE,
                       password_hash TEXT NOT NULL,
                       role          TEXT NOT NULL DEFAULT 'member'
                           CHECK (role IN ('admin', 'owner', 'member')),
                       kyc_status    TEXT NOT NULL DEFAULT 'none'
                           CHECK (kyc_status IN ('none', 'pending', 'verified', 'rejected')),
                       referral_code TEXT NOT NULL UNIQUE,
                       referred_by   UUID REFERENCES users(id),
                       created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

