import unittest
from check_boundaries import MODULE, decode_stream, violations

def package(path, imports=(), **extra):
    return {"ImportPath":path, "Imports":list(imports), **extra}

class Boundaries(unittest.TestCase):
    def test_valid_graph(self):
        graph=[package("net/http",Standard=True),package(MODULE+"/cmd/northway",[MODULE+"/internal/app"]),package(MODULE+"/internal/app",[MODULE+"/internal/httpapi","net/http"])]
        self.assertEqual([],violations(graph))

    def test_rejects_transport_storage_bypass_including_external_tests(self):
        for path in [MODULE+"/internal/httpapi",MODULE+"/internal/httpapi_test ["+MODULE+"/internal/httpapi.test]"]:
            with self.subTest(path=path):
                self.assertTrue(violations([package(path,[MODULE+"/internal/sqlite/sqlc"])]))

    def test_rejects_feature_provider_dependency(self):
        self.assertTrue(violations([package(MODULE+"/internal/ranking",[MODULE+"/internal/providers/anthropic"])]))

    def test_sdk_only_in_adapter(self):
        sdk="github.com/anthropics/anthropic-sdk-go"
        self.assertTrue(violations([package(MODULE+"/internal/httpapi",[sdk])]))
        self.assertEqual([],violations([package(MODULE+"/internal/providers/anthropic",[sdk])]))

    def test_rejects_unregistered_layers_and_legacy_imports(self):
        self.assertTrue(violations([package(MODULE+"/internal/misc")]))
        self.assertTrue(violations([package(MODULE)]))
        self.assertTrue(violations([package(MODULE+"/internal/app",["github.com/jonesrussell/north-cloud/infrastructure"]) ]))

    def test_command_cannot_bypass_app(self):
        self.assertTrue(violations([package(MODULE+"/cmd/northway",[MODULE+"/internal/httpapi"])]))

    def test_standard_sql_cannot_bypass_storage_adapter(self):
        graph=[package("database/sql",Standard=True),package(MODULE+"/internal/httpapi",["database/sql"])]
        self.assertTrue(violations(graph))

    def test_json_stream(self):
        self.assertEqual([{"n":1},{"n":2}],list(decode_stream(' {"n":1}\n {"n":2}\n')))

if __name__=="__main__":unittest.main()
