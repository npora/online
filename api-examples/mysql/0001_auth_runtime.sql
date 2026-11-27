-- Shared Go/Node backend runtime tables.
-- Apply after the template database/mysql SQL in a new example database.
-- The template intentionally leaves authentication/session storage to the backend.
CREATE TABLE `sys_auth_session` (
  `token_hash` CHAR(64) NOT NULL,
  `public_id` CHAR(32) NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `credential_fingerprint` CHAR(64) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `expires_at` BIGINT NOT NULL,
  PRIMARY KEY (`token_hash`),
  UNIQUE KEY `uk_sys_auth_session_public_id` (`public_id`),
  KEY `idx_sys_auth_session_user_expiry` (`user_id`, `expires_at`, `created_at`),
  CONSTRAINT `fk_sys_auth_session_user` FOREIGN KEY (`user_id`) REFERENCES `sys_user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_auth_attempt` (
  `bucket` CHAR(64) NOT NULL,
  `attempts` INT UNSIGNED NOT NULL DEFAULT 0,
  `expires_at` BIGINT NOT NULL,
  PRIMARY KEY (`bucket`),
  KEY `idx_sys_auth_attempt_expiry` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
