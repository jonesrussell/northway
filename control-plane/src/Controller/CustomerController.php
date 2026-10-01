<?php
declare(strict_types=1);
namespace App\Controller;
use App\Infrastructure\NewsClient;
use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use Waaseyaa\Access\AccountInterface;
use Waaseyaa\Access\User\UserInternalFieldReaderInterface;
use Waaseyaa\Entity\EntityTypeManagerInterface;
use Waaseyaa\User\User;

final readonly class CustomerController
{
    public function __construct(private EntityTypeManagerInterface $entities,private UserInternalFieldReaderInterface $fields,private NewsClient $api) {}
    public function handle(Request $request,string $method,string $path): Response
    {
        $headers=['Content-Type'=>'application/json','Cache-Control'=>'no-store','X-Content-Type-Options'=>'nosniff'];
        $account=$request->attributes->get('_account');
        if (!$account instanceof AccountInterface || !$account->isAuthenticated() || (int)$account->id()<=0) {return new Response('{"message":"Sign in required."}',401,$headers);}
        $user=$this->entities->getRepository('user')->find((string)$account->id());
        // Waaseyaa's authenticated route resolves active accounts on each request.
        // Email addresses are unverified in this invitation beta.
        if (!$user instanceof User) {return new Response('{"message":"An active account is required."}',403,$headers);}
        $credentials=$this->fields->credentials($user);
        $generation=$_SESSION['northcloud_credential_generation']??'';
        if (!$credentials->active || $credentials->passwordHash==='' || !is_string($generation) || !hash_equals(hash('sha256',$credentials->passwordHash),$generation)) {
            unset($_SESSION['waaseyaa_uid'],$_SESSION['northcloud_credential_generation']);
            return new Response('{"message":"Sign in again."}',401,$headers);
        }
        if ($request->query->count()!==0 || strlen($request->getContent())>32768) {return new Response('{"message":"Invalid request."}',400,$headers);}
        try {
            $body=$request->getContent();
            if ($body!=='' && $request->headers->get('Content-Type')!=='application/json') {return new Response('{"message":"JSON required."}',415,$headers);}
            $response=$this->api->request($user->uuid(),$method,$path,$body===''?null:$body,$request->headers->get('Idempotency-Key'));
            // Only an allowlisted application body crosses back. Never forward cookies/auth headers.
            if ($response->statusCode===204) {return new Response('',204,$headers);}
            json_decode($response->body,true,64,JSON_THROW_ON_ERROR);
            if ($response->statusCode===429) {$headers['Retry-After']='60';}
            return new Response($response->body,$response->statusCode,$headers);
        } catch (\InvalidArgumentException|\JsonException) {return new Response('{"message":"Invalid request or response."}',400,$headers);}
        catch (\Throwable) {return new Response('{"message":"News service temporarily unavailable."}',503,$headers);}
    }
}
