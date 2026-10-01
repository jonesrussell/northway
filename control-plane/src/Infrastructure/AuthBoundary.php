<?php
declare(strict_types=1);
namespace App\Infrastructure;

use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use Waaseyaa\Access\User\UserInternalFieldReaderInterface;
use Waaseyaa\Entity\EntityTypeManagerInterface;
use Waaseyaa\User\User;

/** Single-host adapter around maintained framework controllers; no password handling. */
final readonly class AuthBoundary
{
    public function __construct(private EntityTypeManagerInterface $entities, private UserInternalFieldReaderInterface $fields, private string $storage) {}

    /** Invite membership is not proof of email ownership. Use explicit sign-in
     * after registration; never retain the framework's provisional invite session.
     * Caller holds the shared token mutation lock throughout this operation.
     */
    public function register(Request $request, callable $controller): Response
    {
        $response=$controller($request);
        if ($response->getStatusCode()!==201) {return $response;}
        \Waaseyaa\User\Session\AuthenticatedSession::clearIdentity();
        unset($_SESSION['northcloud_credential_generation']);
        $body=json_decode((string)$response->getContent(),true,16,JSON_THROW_ON_ERROR);
        $repository=$this->entities->getRepository('user');
        $user=$repository->find((string)($body['data']['id']??''));
        if (!$user instanceof User) {throw new \RuntimeException('Registered account unavailable.');}
        $user->setEmailVerified(false);
        $repository->save($user);
        $body['data']['email_verified']=false;
        $body['meta']['verification_required']=false;
        $response->setContent(json_encode($body,JSON_THROW_ON_ERROR));
        return $response;
    }

    public function login(Request $request, callable $controller): Response
    {
        unset($_SESSION['northcloud_credential_generation']);
        $response=$controller($request);
        $id=$_SESSION['waaseyaa_uid']??null;
        if ($response->getStatusCode()===200 && $id!==null) {
            $user=$this->entities->getRepository('user')->find((string)$id);
            if ($user instanceof User) {
                $credentials=$this->fields->credentials($user);
                if ($credentials->active && $credentials->passwordHash!=='') {
                    $_SESSION['northcloud_credential_generation']=hash('sha256',$credentials->passwordHash);
                }
            }
        }
        return $response;
    }

    /** Serialize token validation + mutation + consumption across PHP workers.
     * The private shared app volume is required; multi-host operation is unsupported.
     * Nonblocking lock bounds contention; fail closed on unavailable storage.
     */
    public function tokenMutation(Request $request, callable $controller): Response
    {
        $old=umask(0077);
        $lock=fopen($this->storage.'/northcloud-auth.lock','c');
        umask($old);
        if ($lock===false) {return new Response('{"error":"unavailable"}',503,['Content-Type'=>'application/json']);}
        try {
            if (!flock($lock,LOCK_EX|LOCK_NB)) {return new Response('{"error":"retry_later"}',503,['Content-Type'=>'application/json','Retry-After'=>'1']);}
            return $controller($request);
        } finally {flock($lock,LOCK_UN);fclose($lock);}
    }
}
