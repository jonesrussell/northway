<?php
declare(strict_types=1);
// Local operator command. Input JSON on stdin; token output goes only to a new
// private file. Never put credentials in arguments, stdout, logs or shell history.
use Waaseyaa\Foundation\Kernel\ConsoleKernel;
use Waaseyaa\Auth\Token\AuthTokenRepositoryInterface;
use Waaseyaa\User\User;

require dirname(__DIR__).'/vendor/autoload.php';
if (PHP_SAPI!=='cli') {exit(1);}
try {
    $raw=stream_get_contents(STDIN,8193);
    if ($raw===false || strlen($raw)>8192) {throw new RuntimeException('Invalid input.');}
    $input=json_decode($raw,true,8,JSON_THROW_ON_ERROR);
    $action=$input['action']??'';
    if (!in_array($action,['invite','activate','recovery','export','suspend','delete'],true)) {throw new RuntimeException('Unsupported account operation.');}
    $output=null;
    if (in_array($action,['invite','recovery','export'],true)) {
        $path=$input['output_file']??'';$dir=realpath(dirname($path));
        if ($dir===false || is_link(dirname($path)) || (fileperms($dir)&0077)!==0) {throw new RuntimeException('Output directory must be private (0700).');}
        $old=umask(0077);$output=fopen($path,'x');umask($old);
        if ($output===false) {throw new RuntimeException('Use a new private token file.');}
    }
    $kernel=new ConsoleKernel(dirname(__DIR__));$kernel->bootForCli();
    $tokens=$kernel->buildHandlerContainer()->get(AuthTokenRepositoryInterface::class);
    if (!$tokens instanceof AuthTokenRepositoryInterface) {throw new RuntimeException('Token service unavailable.');}
    if ($action==='invite') {
        $token=$tokens->createToken(null,'invite',86400);
        fwrite($output,$token."\n");fclose($output);echo "Invitation written to the private output file; expires in 24 hours.\n";exit(0);
    }
    if (($input['identity_verified']??false)!==true) {throw new RuntimeException('Operator identity verification is required.');}
    $id=$input['account_id']??'';
    if (!is_string($id) || preg_match('/\A[1-9][0-9]*\z/D',$id)!==1) {throw new RuntimeException('Explicit account ID required.');}
    $repository=$kernel->getEntityTypeManager()->getRepository('user');$user=$repository->find($id);
    if (!$user instanceof User) {throw new RuntimeException('Account unavailable.');}
    if ($action==='activate') {$user->setEmailVerified(true)->setActive(true);$repository->save($user);echo "Verified account activated.\n";}
    elseif ($action==='recovery') {$token=$tokens->createToken($id,'password_reset',3600);fwrite($output,$token."\n");fclose($output);echo "Recovery token written to the private output file; expires in one hour.\n";}
    elseif ($action==='export') {
        $fields=$kernel->buildHandlerContainer()->get(\Waaseyaa\Access\User\UserInternalFieldReaderInterface::class);
        $identity=$fields->sessionIdentity($user);
        fwrite($output,json_encode(['schema'=>'northcloud.account-export.v1','account_id'=>$id,'workspace_uuid'=>$user->uuid(),'name'=>$identity->name,'email'=>$identity->mail,'roles'=>$identity->roles,'email_verified'=>$fields->verification($user)->emailVerified],JSON_THROW_ON_ERROR)."\n");fclose($output);echo "Account export written privately.\n";
    } else {
        if (($input['confirm_account_id']??null)!==$id || ($input['workspace_uuid']??null)!==$user->uuid()) {throw new RuntimeException('Exact account and workspace confirmation required.');}
        if ($action==='delete' && ($input['data_plane_deleted']??false)!==true) {throw new RuntimeException('Delete the exact Go workspace first.');}
        $user->setActive(false);$repository->save($user);$tokens->revokeTokensForUser($id);
        if ($action==='delete') {$repository->delete($user);}
        echo "Account lifecycle operation completed.\n";
    }
} catch(Throwable) {fwrite(STDERR,"Account operation failed. Check the private input and deployment readiness; no credential was printed.\n");exit(1);}
