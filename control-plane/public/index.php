<?php
declare(strict_types=1);
use Waaseyaa\Foundation\Kernel\HttpKernel;

$root = dirname(__DIR__);
require $root . '/vendor/autoload.php';
// Expose only the beta's supported routes. Framework 2FA/OIDC/admin and generic
// entity APIs are not this application's customer contract.
$path=parse_url($_SERVER['REQUEST_URI']??'/',PHP_URL_PATH);
$allowed=in_array($path,['/','/login','/register','/app','/reset-password','/app.js','/app.css',\Waaseyaa\Foundation\Diagnostic\CleanUrlProbe::PATH,'/api/auth/login','/api/auth/register','/api/auth/logout','/api/auth/reset-password'],true)
    || (is_string($path) && preg_match('~\A/api/customer/(?:workspace|keys(?:/[A-Za-z0-9_-]{1,80})?|feeds|feed-queries|snapshots/[a-f0-9-]{36}|feedback)\z~D',$path)===1);
if (!$allowed) {http_response_code(404);header('Cache-Control: no-store');exit;}
// Production configuration comes from the process/secret mount, not web files.
if (PHP_SAPI === 'cli-server') {
    $path = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH);
    if (in_array($path, ['/app.js', '/app.css'], true)) { return false; }
}
try { (new HttpKernel($root))->handle()->send(); }
catch (Throwable) {
    http_response_code(503);
    header('Content-Type: application/json');
    header('Cache-Control: no-store');
    echo '{"message":"NorthCloud is temporarily unavailable."}';
}
