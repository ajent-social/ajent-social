-- Shared quotas across the rolling fleet. No chat text or raw IP addresses.
CREATE TABLE chat_usage (
 bucket TIMESTAMPTZ NOT NULL,
 subject TEXT NOT NULL,
 count INTEGER NOT NULL DEFAULT 1,
 PRIMARY KEY(bucket,subject)
);
