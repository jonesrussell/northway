<?php
declare(strict_types=1);
return [
    'environment' => getenv('APP_ENV') ?: 'production',
    'debug' => false,
    'log_level' => 'warning',
    'database' => null,
    'api' => ['entity_type_allowlist' => []],
    'ai_catalog' => ['enabled' => false],
    'api_catalog' => ['enabled' => false],
    'auth' => ['registration' => 'invite', 'require_verified_email' => true, 'dev_fallback_account' => false, 'mail_missing_policy' => 'fail'],
    'api_keys' => [],
    'jwt_secret' => '',
    'cors_origins' => [],
    'diagnostics' => ['clean_url_probe_url' => getenv('APP_URL') ?: ''],
    'northcloud' => [
        'api_url' => getenv('NORTHCLOUD_API_URL') ?: '',
        'issuer' => getenv('APP_URL') ?: '',
        'audience' => 'northcloud-api',
        'key_id' => getenv('NORTHCLOUD_SIGNING_KEY_ID') ?: '',
        'seed' => getenv('NORTHCLOUD_SIGNING_SEED') ?: '',
    ],
];
