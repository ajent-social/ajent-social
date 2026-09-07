-- Durable admission limits apply across application replicas. No raw IPs or tokens.
CREATE TABLE signup_buckets (
  bucket DATE NOT NULL,
  subject TEXT NOT NULL,
  count INTEGER NOT NULL,
  PRIMARY KEY (bucket, subject)
);
CREATE TABLE public_enrollments (
  credential_id UUID PRIMARY KEY REFERENCES credentials(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
