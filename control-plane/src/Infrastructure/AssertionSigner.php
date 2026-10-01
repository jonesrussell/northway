<?php
declare(strict_types=1);
namespace App\Infrastructure;

use Symfony\Component\Uid\Uuid;

final readonly class AssertionSigner
{
    private string $key;
    public function __construct(private string $issuer, private string $audience, private string $keyId, #[\SensitiveParameter] string $seed)
    {
        $decoded = base64_decode($seed, true);
        if ($issuer === '' || strlen($issuer)>256 || $audience === '' || strlen($audience)>256 || $keyId === '' || strlen($keyId)>64 || $decoded === false || strlen($decoded)!==32) { throw new \InvalidArgumentException('Incomplete NorthCloud signing configuration.'); }
        $this->key = sodium_crypto_sign_secretkey(sodium_crypto_sign_seed_keypair($decoded));
    }
    public function issue(string $accountUuid, string $operation): string
    {
        if (preg_match('/\A[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\z/D', $accountUuid)!==1) { throw new \InvalidArgumentException('Invalid account identity.'); }
        $now=time();
        $header=['alg'=>'EdDSA','typ'=>'ncl-fpa+jwt','kid'=>$this->keyId];
        $claims=['iss'=>$this->issuer,'aud'=>$this->audience,'sub'=>$accountUuid,'tenant'=>$accountUuid,'iat'=>$now,'nbf'=>$now,'exp'=>$now+30,'jti'=>Uuid::v4()->toRfc4122(),'op'=>$operation,'ver'=>1];
        $message=self::encode(json_encode($header,JSON_THROW_ON_ERROR)).'.'.self::encode(json_encode($claims,JSON_THROW_ON_ERROR));
        return $message.'.'.self::encode(sodium_crypto_sign_detached($message,$this->key));
    }
    private static function encode(string $value): string { return rtrim(strtr(base64_encode($value),'+/','-_'),'='); }
    public function __debugInfo(): array { return ['signing'=>'[REDACTED]']; }
}
