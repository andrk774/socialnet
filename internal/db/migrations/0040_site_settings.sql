CREATE TABLE site_settings(id boolean PRIMARY KEY DEFAULT true CHECK(id), public_url text NOT NULL, ingress_class text NOT NULL, tls_secret text NOT NULL);
