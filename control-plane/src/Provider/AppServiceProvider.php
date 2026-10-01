<?php
declare(strict_types=1);
namespace App\Provider;
use App\Controller\CustomerController;
use App\Infrastructure\AssertionSigner;
use App\Infrastructure\NewsClient;
use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use Waaseyaa\Access\User\UserInternalFieldReaderInterface;
use Waaseyaa\Entity\EntityTypeManager;
use Waaseyaa\Foundation\ServiceProvider\ServiceProvider;
use Waaseyaa\Foundation\Diagnostic\CleanUrlProbe;
use Waaseyaa\HttpClient\StreamHttpClient;
use Waaseyaa\Routing\RouteBuilder;
use Waaseyaa\Routing\WaaseyaaRouter;
use Waaseyaa\Auth\Extension\ProvidesAuthExtensionsInterface;
use Waaseyaa\Auth\Extension\AuthExtensionContribution;
use App\Domain\InvitationApproval;

final class AppServiceProvider extends ServiceProvider implements ProvidesAuthExtensionsInterface
{
    public function authExtensions(): AuthExtensionContribution { return new AuthExtensionContribution(registration:new InvitationApproval()); }
    public function register(): void
    {
        $this->singleton(AssertionSigner::class,fn()=>new AssertionSigner($this->config['northcloud']['issuer'],$this->config['northcloud']['audience'],$this->config['northcloud']['key_id'],$this->config['northcloud']['seed']));
        $this->singleton(NewsClient::class,fn()=>new NewsClient($this->config['northcloud']['api_url'],$this->resolve(AssertionSigner::class),new StreamHttpClient(timeout:12.0,maxResponseBytes:1024*1024)));
        $this->singleton(CustomerController::class,fn()=>new CustomerController($this->resolve(EntityTypeManager::class),$this->resolve(UserInternalFieldReaderInterface::class),$this->resolve(NewsClient::class)));
        foreach (['register','login','reset-password'] as $name) {
            $this->singleton('northcloud.auth.'.$name,fn()=> $this->authHandlers()[$name]);
        }
    }
    private function authHandlers(): array
    {
        // Explicit app routes wrap the maintained controllers in the beta's
        // single-host mutation lock and session-generation stamp.
        $entities=$this->resolve(EntityTypeManager::class);
        $fields=$this->resolve(UserInternalFieldReaderInterface::class);
        $tokens=$this->resolve(\Waaseyaa\Auth\Token\AuthTokenRepositoryInterface::class);
        $rates=$this->resolve(\Waaseyaa\Auth\RateLimiterInterface::class);
        $lookup=$this->resolve(\Waaseyaa\Access\User\UserIdentityLookupInterface::class);
        $extensions=$this->resolve(\Waaseyaa\Auth\Extension\AuthExtensionRegistry::class);
        $eligibility=$this->resolve(\Waaseyaa\User\Authentication\AuthenticationEligibilityInterface::class);
        $boundary=new \App\Infrastructure\AuthBoundary($entities,$fields,dirname(__DIR__,2).'/storage');
        $register=new \Waaseyaa\Auth\Controller\RegisterController($this->resolve(\Waaseyaa\Auth\Config\AuthConfig::class),$entities,$tokens,$this->resolve(\Waaseyaa\User\AuthMailer::class),$rates,$lookup,$fields,$eligibility,extensions:$extensions);
        $login=new \Waaseyaa\Auth\Controller\LoginController($entities,$rates,$this->resolve(\Waaseyaa\Auth\TwoFactorService::class),$lookup,$fields,$eligibility,extensions:$extensions,passwords:$this->resolve(\Waaseyaa\Auth\Password\LegacyPasswordUpgrade::class));
        $reset=new \Waaseyaa\Auth\Controller\ResetPasswordController($entities,$tokens,$fields);
        return ['register'=>fn(Request $r)=>$boundary->tokenMutation($r,$register),'login'=>fn(Request $r)=>$boundary->tokenMutation($r,fn(Request $request)=>$boundary->login($request,$login)),'reset-password'=>fn(Request $r)=>$boundary->tokenMutation($r,$reset)];
    }
    public function routes(WaaseyaaRouter $router,?EntityTypeManager $entityTypeManager=null): void
    {
        foreach (['register','login','reset-password'] as $name) {
            $handler=fn(Request $r)=>($this->resolve('northcloud.auth.'.$name))($r);
            $router->addRoute('northcloud.auth.'.$name,RouteBuilder::create('/api/auth/'.$name)->controller($handler)->methods('POST')->priority(100)->allowAll()->requireCsrf()->build());
        }
        foreach (['/'=>'home','/login'=>'login','/register'=>'register','/app'=>'app','/reset-password'=>'reset-password'] as $path=>$page) {
            $route=RouteBuilder::create($path)->controller(fn()=>new Response((string)file_get_contents(dirname(__DIR__,2).'/templates/'.$page.'.html'),200,['Content-Type'=>'text/html; charset=UTF-8','Cache-Control'=>'no-store','Content-Security-Policy'=>"default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",'Referrer-Policy'=>'no-referrer','X-Content-Type-Options'=>'nosniff']))->methods('GET');
            $router->addRoute('northcloud.page.'.$page,($page==='app'?$route->requireAuthentication():$route->allowAll())->build());
        }
        foreach ([['PUT','workspace'],['GET','keys'],['POST','keys'],['DELETE','keys/{id}'],['GET','feeds'],['POST','feed-queries'],['GET','snapshots/{id}'],['POST','feedback']] as [$method,$relative]) {
            $route=RouteBuilder::create('/api/customer/'.$relative)->controller(function(Request $request,string ...$parameters)use($method,$relative){
                $path='/v1/'.str_replace('{id}',(string)$request->attributes->get('id',''),$relative);
                return $this->resolve(CustomerController::class)->handle($request,$method,$path);
            })->methods($method)->requireAuthentication();
            if ($method!=='GET') {$route->requireCsrf();}
            $router->addRoute('northcloud.customer.'.$method.'.'.$relative,$route->build());
        }
        $router->addRoute('northcloud.probe',RouteBuilder::create(CleanUrlProbe::PATH)->controller(fn()=>new Response(CleanUrlProbe::SENTINEL,200,['Content-Type'=>'text/plain']))->methods('GET')->allowAll()->build());
    }
}
