<?php
declare(strict_types=1);
namespace App\Infrastructure;
use Waaseyaa\HttpClient\HttpClientInterface;
use Waaseyaa\HttpClient\HttpResponse;

final readonly class NewsClient
{
    public function __construct(private string $origin,private AssertionSigner $signer,private HttpClientInterface $http)
    {
        $p=parse_url($origin);
        if (!is_array($p) || !isset($p['scheme'],$p['host']) || isset($p['user']) || isset($p['pass']) || isset($p['query']) || isset($p['fragment']) || !in_array($p['path']??'',['','/'],true) || !($p['scheme']==='https' || ($p['scheme']==='http' && in_array($p['host'],['127.0.0.1','localhost'],true)))) { throw new \InvalidArgumentException('API origin must be HTTPS or local loopback HTTP.'); }
    }
    public function request(string $account,string $method,string $path,?string $body=null,?string $idempotency=null): HttpResponse
    {
        $allowed=match($method) {
            'PUT' => $path==='/v1/workspace',
            'GET' => in_array($path,['/v1/keys','/v1/feeds'],true) || preg_match('#\A/v1/snapshots/[0-9a-f-]{36}\z#D',$path)===1,
            'POST' => in_array($path,['/v1/keys','/v1/feed-queries','/v1/feedback'],true),
            'DELETE' => preg_match('#\A/v1/keys/[0-9a-f]{32}\z#D',$path)===1,
            default => false,
        };
        if (!$allowed || ($body!==null && strlen($body)>32768)) { throw new \InvalidArgumentException('Unsupported NorthCloud request.'); }
        $headers=['Authorization'=>'Bearer '.$this->signer->issue($account,$method.' '.$path),'Accept'=>'application/json'];
        if ($body!==null) { $headers['Content-Type']='application/json'; }
        if ($idempotency!==null) {
            if (preg_match('/\A[a-zA-Z0-9_-]{16,128}\z/D',$idempotency)!==1) { throw new \InvalidArgumentException('Invalid idempotency key.'); }
            $headers['Idempotency-Key']=$idempotency;
        }
        return $this->http->request($method,rtrim($this->origin,'/').$path,$headers,$body);
    }
}
