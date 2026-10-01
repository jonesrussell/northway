<?php
declare(strict_types=1);
namespace App\Tests\Unit;
use App\Controller\CustomerController;
use App\Infrastructure\AssertionSigner;
use App\Infrastructure\NewsClient;
use App\Domain\InvitationApproval;
use PHPUnit\Framework\TestCase;
use Symfony\Component\HttpFoundation\Request;
use Waaseyaa\Access\AccountInterface;
use Waaseyaa\Access\User\UserInternalFieldReaderInterface;
use Waaseyaa\Access\User\UserVerificationSnapshot;
use Waaseyaa\Auth\Extension\RegistrationContext;
use Waaseyaa\Entity\EntityTypeManagerInterface;
use Waaseyaa\Entity\Repository\EntityRepositoryInterface;
use Waaseyaa\HttpClient\HttpClientInterface;
use Waaseyaa\HttpClient\HttpResponse;
use Waaseyaa\User\User;

final class CustomerBoundaryTest extends TestCase
{
    private const UUID='00000000-0000-4000-8000-000000000001';
    private function signer(): AssertionSigner {return new AssertionSigner('https://northcloud.one','northcloud-api','test',base64_encode(str_repeat('t',32)));}
    public function testSignatureProfileAndOperationAreProductSpecific(): void
    {
        $token=$this->signer()->issue(self::UUID,'GET /v1/keys');$parts=explode('.',$token);
        $decode=static fn(string $v)=>base64_decode(strtr($v,'-_','+/'),true);
        self::assertTrue(sodium_crypto_sign_verify_detached($decode($parts[2]),$parts[0].'.'.$parts[1],sodium_crypto_sign_publickey(sodium_crypto_sign_seed_keypair(str_repeat('t',32)))));
        $header=json_decode($decode($parts[0]),true,8,JSON_THROW_ON_ERROR);$claims=json_decode($decode($parts[1]),true,8,JSON_THROW_ON_ERROR);
        self::assertSame('ncl-fpa+jwt',$header['typ']);self::assertSame('northcloud-api',$claims['aud']);self::assertSame(self::UUID,$claims['tenant']);self::assertSame(self::UUID,$claims['sub']);self::assertSame('GET /v1/keys',$claims['op']);self::assertSame(30,$claims['exp']-$claims['iat']);
        self::assertNotSame($token,$this->signer()->issue(self::UUID,'GET /v1/keys'));
    }
    public function testTransportRejectsRedirectAndPathInjectionInputs(): void
    {
        $http=$this->createMock(HttpClientInterface::class);$http->expects(self::never())->method('request');
        $client=new NewsClient('http://127.0.0.1:8080',$this->signer(),$http);
        $this->expectException(\InvalidArgumentException::class);$client->request(self::UUID,'GET','/v1/keys?tenant=other');
    }
    public function testControllerDerivesIdentityAndDoesNotForwardUpstreamHeaders(): void
    {
        $user=new User(['uid'=>42,'uuid'=>self::UUID,'status'=>true,'email_verified'=>true]);
        $repository=$this->createMock(EntityRepositoryInterface::class);$repository->expects(self::once())->method('find')->with('42')->willReturn($user);
        $entities=$this->createMock(EntityTypeManagerInterface::class);$entities->expects(self::once())->method('getRepository')->with('user')->willReturn($repository);
        $fields=$this->createStub(UserInternalFieldReaderInterface::class);$fields->method('verification')->willReturn(new UserVerificationSnapshot('fixture@example.test',true,true));
        $fields->method('credentials')->willReturn(new \Waaseyaa\Access\User\UserCredentialSnapshot(true,'test-hash'));
        $_SESSION['northcloud_credential_generation']=hash('sha256','test-hash');
        $http=$this->createMock(HttpClientInterface::class);$http->expects(self::once())->method('request')->with('GET','http://127.0.0.1:8080/v1/keys',self::callback(function(array $headers):bool{
            $parts=explode('.',substr($headers['Authorization'],7));$c=json_decode(base64_decode(strtr($parts[1],'-_','+/')),true);return $c['tenant']===self::UUID && $c['sub']===self::UUID;
        }),null)->willReturn(new HttpResponse(200,'{"keys":[]}', ['Set-Cookie'=>'unsafe=1','Authorization'=>'unsafe']));
        $account=$this->createStub(AccountInterface::class);$account->method('isAuthenticated')->willReturn(true);$account->method('id')->willReturn(42);
        $request=Request::create('/api/customer/keys');$request->attributes->set('_account',$account);$request->attributes->set('tenant','00000000-0000-4000-8000-000000000002');
        $controller=new CustomerController($entities,$fields,new NewsClient('http://127.0.0.1:8080',$this->signer(),$http));$response=$controller->handle($request,'GET','/v1/keys');
        self::assertSame(200,$response->getStatusCode());self::assertFalse($response->headers->has('Set-Cookie'));self::assertFalse($response->headers->has('Authorization'));self::assertStringContainsString('no-store',(string)$response->headers->get('Cache-Control'));
    }
    public function testUnauthenticatedRequestNeverTouchesDataPlane(): void
    {
        $http=$this->createMock(HttpClientInterface::class);$http->expects(self::never())->method('request');$entities=$this->createMock(EntityTypeManagerInterface::class);$entities->expects(self::never())->method('getRepository');
        $controller=new CustomerController($entities,$this->createStub(UserInternalFieldReaderInterface::class),new NewsClient('http://127.0.0.1:8080',$this->signer(),$http));
        self::assertSame(401,$controller->handle(Request::create('/api/customer/keys'),'GET','/v1/keys')->getStatusCode());
    }
    public function testInvitationRequiresOperatorApproval(): void
    {
        $decision=(new InvitationApproval())->decide(new RegistrationContext('Fixture','fixture@example.test','invite'));self::assertTrue($decision->allowed);self::assertTrue($decision->requiresApproval);
    }
}
