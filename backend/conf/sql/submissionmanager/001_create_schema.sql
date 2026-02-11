SET ANSI_NULLS ON;
SET QUOTED_IDENTIFIER ON;

IF OBJECT_ID('dbo.submission_intents', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.submission_intents (
    intent_id NVARCHAR(200) NOT NULL PRIMARY KEY,
    submission_target NVARCHAR(200) NOT NULL,
    payload VARBINARY(MAX) NOT NULL,
    payload_hash BINARY(32) NOT NULL,
    gateway_type NVARCHAR(32) NOT NULL,
    gateway_url NVARCHAR(512) NOT NULL,
    policy NVARCHAR(32) NOT NULL,
    max_acceptance_seconds INT NULL,
    max_attempts INT NULL,
    delivery_tracking_mode NVARCHAR(8) NOT NULL,
    delivery_tracking_stale_after_seconds INT NULL,
    terminal_outcomes NVARCHAR(MAX) NOT NULL,
    webhook_url NVARCHAR(512) NULL,
    webhook_headers NVARCHAR(MAX) NULL,
    webhook_headers_env NVARCHAR(MAX) NULL,
    webhook_secret_env NVARCHAR(256) NULL,
    webhook_status NVARCHAR(32) NULL,
    webhook_attempted_at DATETIME2(7) NULL,
    webhook_delivered_at DATETIME2(7) NULL,
    webhook_error NVARCHAR(512) NULL,
    status NVARCHAR(32) NOT NULL,
    final_outcome_status NVARCHAR(32) NULL,
    final_outcome_reason NVARCHAR(64) NULL,
    exhausted_reason NVARCHAR(64) NULL,
    -- attempt_count is the authoritative attempt number source.
    attempt_count INT NOT NULL DEFAULT 0,
    created_at DATETIME2(7) NOT NULL,
    updated_at DATETIME2(7) NOT NULL,
    last_modified_at DATETIME2(7) NOT NULL,
    next_attempt_at DATETIME2(7) NULL
  );
END;

IF OBJECT_ID('dbo.submission_manager_leases', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.submission_manager_leases (
    lease_name NVARCHAR(64) NOT NULL PRIMARY KEY,
    holder_id NVARCHAR(128) NOT NULL,
    lease_epoch BIGINT NOT NULL,
    acquired_at DATETIME2(7) NOT NULL,
    renewed_at DATETIME2(7) NOT NULL,
    expires_at DATETIME2(7) NOT NULL
  );
END;

IF COL_LENGTH('dbo.submission_intents', 'mode') IS NOT NULL
BEGIN
  ALTER TABLE dbo.submission_intents DROP COLUMN mode;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_url') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_url NVARCHAR(512) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_headers') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_headers NVARCHAR(MAX) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_headers_env') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_headers_env NVARCHAR(MAX) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_secret_env') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_secret_env NVARCHAR(256) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_status') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_status NVARCHAR(32) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_attempted_at') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_attempted_at DATETIME2(7) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_delivered_at') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_delivered_at DATETIME2(7) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'webhook_error') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD webhook_error NVARCHAR(512) NULL;
END;

IF COL_LENGTH('dbo.submission_intents', 'delivery_tracking_mode') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD delivery_tracking_mode NVARCHAR(8) NOT NULL
    CONSTRAINT DF_submission_intents_delivery_tracking_mode DEFAULT 'off';
END;

IF COL_LENGTH('dbo.submission_intents', 'delivery_tracking_stale_after_seconds') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents ADD delivery_tracking_stale_after_seconds INT NULL;
END;

UPDATE dbo.submission_intents
SET delivery_tracking_mode = 'off'
WHERE delivery_tracking_mode IS NULL
   OR LTRIM(RTRIM(delivery_tracking_mode)) = ''
   OR delivery_tracking_mode NOT IN ('on', 'off');

UPDATE dbo.submission_intents
SET delivery_tracking_stale_after_seconds = NULL
WHERE delivery_tracking_mode = 'off';

IF COL_LENGTH('dbo.submission_intents', 'last_modified_at') IS NULL
BEGIN
  ALTER TABLE dbo.submission_intents
    ADD last_modified_at DATETIME2(7) NOT NULL
      CONSTRAINT DF_submission_intents_last_modified_at DEFAULT SYSUTCDATETIME();
END;

IF OBJECT_ID('dbo.submission_attempts', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.submission_attempts (
    intent_id NVARCHAR(200) NOT NULL,
    attempt_number INT NOT NULL,
    started_at DATETIME2(7) NOT NULL,
    finished_at DATETIME2(7) NOT NULL,
    outcome_status NVARCHAR(32) NULL,
    outcome_reason NVARCHAR(64) NULL,
    error NVARCHAR(512) NULL,
    CONSTRAINT PK_submission_attempts PRIMARY KEY (intent_id, attempt_number),
    CONSTRAINT FK_submission_attempts_intent FOREIGN KEY (intent_id)
      REFERENCES dbo.submission_intents(intent_id) ON DELETE CASCADE
  );
END;

IF OBJECT_ID('dbo.intent_delivery_state', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.intent_delivery_state (
    intent_id NVARCHAR(200) NOT NULL PRIMARY KEY,
    delivery_status NVARCHAR(32) NOT NULL,
    delivery_freshness NVARCHAR(32) NOT NULL,
    last_non_terminal_effective_at DATETIME2(7) NULL,
    last_non_terminal_received_at DATETIME2(7) NULL,
    last_non_terminal_source_record_id NVARCHAR(200) NULL,
    terminal_locked BIT NOT NULL,
    terminal_status NVARCHAR(32) NULL,
    terminal_effective_at DATETIME2(7) NULL,
    terminal_received_at DATETIME2(7) NULL,
    terminal_source_record_id NVARCHAR(200) NULL,
    stale_after_seconds INT NOT NULL,
    stale_at DATETIME2(7) NULL,
    created_at DATETIME2(7) NOT NULL,
    updated_at DATETIME2(7) NOT NULL,
    CONSTRAINT FK_intent_delivery_state_intent FOREIGN KEY (intent_id)
      REFERENCES dbo.submission_intents(intent_id) ON DELETE CASCADE
  );
END;

IF OBJECT_ID('dbo.intent_delivery_history', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.intent_delivery_history (
    intent_id NVARCHAR(200) NOT NULL,
    history_seq BIGINT IDENTITY(1,1) NOT NULL,
    source_record_id NVARCHAR(200) NOT NULL,
    provider_event_id NVARCHAR(200) NULL,
    signal_class NVARCHAR(32) NOT NULL,
    provider_observed_at DATETIME2(7) NULL,
    effective_at DATETIME2(7) NOT NULL,
    received_at DATETIME2(7) NOT NULL,
    late_observation BIT NOT NULL,
    terminal_conflict BIT NOT NULL,
    created_at DATETIME2(7) NOT NULL,
    CONSTRAINT PK_intent_delivery_history PRIMARY KEY (intent_id, history_seq),
    CONSTRAINT FK_intent_delivery_history_intent FOREIGN KEY (intent_id)
      REFERENCES dbo.submission_intents(intent_id) ON DELETE CASCADE
  );
END;

IF NOT EXISTS (
  SELECT 1
  FROM sys.indexes
  WHERE name = 'uq_intent_delivery_history_source'
    AND object_id = OBJECT_ID('dbo.intent_delivery_history')
)
BEGIN
  CREATE UNIQUE INDEX uq_intent_delivery_history_source
    ON dbo.intent_delivery_history(intent_id, source_record_id);
END;

IF NOT EXISTS (
  SELECT 1
  FROM sys.indexes
  WHERE name = 'idx_submission_intents_next_attempt'
    AND object_id = OBJECT_ID('dbo.submission_intents')
)
BEGIN
  SET ANSI_NULLS ON;
  SET QUOTED_IDENTIFIER ON;
  CREATE INDEX idx_submission_intents_next_attempt
    ON dbo.submission_intents(next_attempt_at)
    WHERE next_attempt_at IS NOT NULL;
END;

IF NOT EXISTS (
  SELECT 1
  FROM sys.indexes
  WHERE name = 'idx_intent_delivery_state_freshness'
    AND object_id = OBJECT_ID('dbo.intent_delivery_state')
)
BEGIN
  CREATE INDEX idx_intent_delivery_state_freshness
    ON dbo.intent_delivery_state(delivery_status, delivery_freshness, stale_at);
END;
