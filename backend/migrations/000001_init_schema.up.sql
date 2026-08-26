CREATE TABLE IF NOT EXISTS categories (
    category_id UUID PRIMARY KEY,
    category TEXT NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE IF NOT EXISTS locations (
    location_id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    city TEXT NOT NULL,
    postal_code TEXT NOT NULL,
    street TEXT NOT NULL,
    house_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE payment_status AS ENUM ('pending', 'paid', 'failed', 'refunded');

CREATE TABLE IF NOT EXISTS payments (
    payment_id UUID PRIMARY KEY,
    amount NUMERIC(12,2) NOT NULL,
    status payment_status NOT NULL DEFAULT 'pending',
    refund_amount NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE user_role AS ENUM ('administrator', 'organizer', 'visitor');
CREATE TYPE user_type AS ENUM ('private', 'organization');

CREATE TABLE IF NOT EXISTS users (
    user_id UUID PRIMARY KEY,
    keycloak_sub TEXT UNIQUE NOT NULL
    --role user_role NOT NULL,  redundant wegen keycloak
    user_type user_type NOT NULL,
    email TEXT NOT NULL UNIQUE,
    phone_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS private_users (
    user_id UUID PRIMARY KEY
    REFERENCES users(user_id)
    ON DELETE SET NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS organization_users (
    user_id UUID PRIMARY KEY
    REFERENCES users(user_id)
    ON DELETE SET NULL,
    organization_name TEXT NOT NULL,
    contact_person_name TEXT
);

CREATE TYPE event_status AS ENUM ('draft', 'published', 'cancelled', 'completed');

CREATE TABLE IF NOT EXISTS events (
    event_id UUID PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    capacity INT NOT NULL,
    status event_status NOT NULL DEFAULT 'draft',
    price NUMERIC(12,2) NOT NULL,
    category_id UUID REFERENCES categories(category_id) ON DELETE SET NULL,
    organizer_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
    location_id UUID REFERENCES locations(location_id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notification_data (
    notification_id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
    subject TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE booking_status AS ENUM ('reserved', 'confirmed', 'cancelled', 'failed');

CREATE TABLE IF NOT EXISTS bookings (
    booking_id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
    event_id UUID REFERENCES events(event_id) ON DELETE SET NULL,
    payment_id UUID REFERENCES payments(payment_id) ON DELETE SET NULL,
    number_of_tickets INT NOT NULL CHECK (number_of_tickets > 0),
    status booking_status NOT NULL DEFAULT 'reserved',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ratings (
    rating_id UUID PRIMARY KEY,
    booking_id UUID REFERENCES bookings(booking_id) ON DELETE SET NULL,
    score INT NOT NULL CHECK (score >= 1 AND score <= 5),
    text TEXT NOT NULL,
    is_visible BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);