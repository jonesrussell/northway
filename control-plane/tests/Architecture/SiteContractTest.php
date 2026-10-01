<?php

declare(strict_types=1);

use PHPUnit\Framework\TestCase;
use Waaseyaa\SiteContract\SiteManifestParser;
use Waaseyaa\SiteContract\SiteManifestSchema;

final class SiteContractTest extends TestCase
{
    public function testGeneratedSiteContractIsPresentAndValid(): void
    {
        $root = dirname(__DIR__, 2);
        $manifest = (string) file_get_contents($root . '/.waaseyaa/site.yaml');
        self::assertNotSame('', $manifest);
        self::assertMatchesRegularExpression('/^[a-f0-9]{64}$/D', new SiteManifestParser()->parse($manifest)->digest);
        self::assertSame(SiteManifestSchema::canonicalJson(), trim((string) file_get_contents($root . '/.waaseyaa/site.schema.json')));
    }
}