<?php

declare(strict_types=1);

use PHPUnit\Framework\TestCase;
use Waaseyaa\SiteContract\SiteManifestParser;

final class SiteGoldenPathTest extends TestCase
{
    public function testProviderNeutralVerificationCommandIsDeclared(): void
    {
        $root = dirname(__DIR__, 2);
        $manifest = new SiteManifestParser()->parse((string) file_get_contents($root . '/.waaseyaa/site.yaml'));
        self::assertSame('bin/maintenance/site-verify', $manifest->verificationCommand);
        $command = $root . '/' . $manifest->verificationCommand;
        self::assertFileExists($command);
        self::assertStringStartsWith('#!/usr/bin/env php', (string) file_get_contents($command));

        // Two properties, measured two separate ways. A hardened
        // execution environment can mount the project tree noexec
        // (e.g. a sandboxed runner's tmpfs): the file is genuinely
        // mode 0755 and owned by the running user, but
        // is_executable() / posix_access(X_OK) both report false
        // because the *mount*, not the inode, denies execution.
        if (DIRECTORY_SEPARATOR === '/') {
            // 1. The artifact carries the POSIX permission bits
            // Waaseyaa promises. fileperms() reads the inode mode
            // directly, which reports the real bits regardless of
            // mount flags — so this still fails the day Framework
            // stops chmod-ing the artifact.
            $mode = fileperms($command);
            self::assertNotFalse($mode, 'unable to stat the generated verification command');
            self::assertSame(0111, $mode & 0111, 'the generated verification command must carry execute permission bits');
        }

        // 2. The command actually runs through the one invocation
        // every caller uses — `PHP_BINARY <script>` (see
        // .ci/site-verify.php, the composer.json `site-verify`
        // script, and the .ci/site-verify exec wrapper). PHP
        // interprets the file's bytes directly; it never asks the
        // filesystem for permission to execute it, so this holds
        // on a noexec mount. A shebang-prefix check alone proves
        // nothing about whether the file actually runs — invoke it.
        $invocation = escapeshellarg(PHP_BINARY) . ' ' . escapeshellarg($command) . ' --self-test';
        exec($invocation . ' 2>&1', $selfTestOutput, $selfTestExitCode);
        self::assertSame(
            0,
            $selfTestExitCode,
            "the generated verification command did not run through `PHP_BINARY <script>`:\n" . implode("\n", $selfTestOutput),
        );
    }
}