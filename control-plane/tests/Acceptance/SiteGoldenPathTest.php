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
        self::assertFileExists($root . '/' . $manifest->verificationCommand);
        self::assertTrue(is_executable($root . '/' . $manifest->verificationCommand));
    }
}