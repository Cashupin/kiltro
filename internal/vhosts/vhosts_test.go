package vhosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkfiles crea carpetas/archivos dentro de base. Las rutas que terminan en "/" son carpetas.
func mkfiles(t *testing.T, base string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(base, filepath.FromSlash(p))
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan(t *testing.T) {
	www := t.TempDir()
	mkfiles(t, www,
		"blog/index.php",
		"Mi_Tienda/",           // se normaliza a mi-tienda
		"api/artisan",          // Laravel
		"api/public/index.php", //
		"solo-artisan/artisan", // sin public/index.php: NO es Laravel
		".git/",                // oculta: se ignora sin avisar
		"mi.tienda/",           // el punto no es válido
		"mi tienda/",           // choca con Mi_Tienda
		"suelto.txt",           // archivo: se ignora
		"Adminer/",             // nombre reservado para la herramienta
	)

	sites, skipped, err := Scan(www)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]Site{}
	for _, s := range sites {
		got[s.Name] = s
	}
	if len(sites) != 4 {
		t.Fatalf("se esperaban 4 sitios, llegaron %d: %+v", len(sites), sites)
	}
	if s := got["api"]; !s.Laravel || s.Root != filepath.Join(www, "api", "public") || s.Domain != "api.test" {
		t.Errorf("api mal detectado: %+v", s)
	}
	if s := got["solo-artisan"]; s.Laravel || s.Root != filepath.Join(www, "solo-artisan") {
		t.Errorf("solo-artisan no debería ser Laravel: %+v", s)
	}
	if _, ok := got["mi-tienda"]; !ok {
		t.Errorf("falta mi-tienda (normalizado de Mi_Tienda)")
	}
	if sites[0].Name != "api" {
		t.Errorf("los sitios deberían venir ordenados, el primero es %q", sites[0].Name)
	}

	if len(skipped) != 3 {
		t.Fatalf("se esperaban 3 ignoradas (Adminer reservado, mi.tienda y el choque), llegaron: %+v", skipped)
	}
	if _, ok := got["adminer"]; ok {
		t.Error("www/Adminer no debería ser un sitio (nombre reservado)")
	}
}

func TestScanSinCarpetaWww(t *testing.T) {
	sites, skipped, err := Scan(filepath.Join(t.TempDir(), "no-existe"))
	if err != nil || len(sites) != 0 || len(skipped) != 0 {
		t.Fatalf("sin www/ debería devolver vacío sin error: %v %v %v", sites, skipped, err)
	}
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	sitesDir := filepath.Join(dir, "sites")
	mkfiles(t, sitesDir, "viejo.test.conf", "no-es-conf.txt")

	sites := []Site{
		{Name: "api", Domain: "api.test", Folder: `C:\www\api`, Root: `C:\www\api\public`, Laravel: true},
		{Name: "blog", Domain: "blog.test", Folder: `C:\www\blog`, Root: `C:\www\blog`},
		{Name: "adminer", Domain: "adminer.test", Root: `C:\etc\adminer`, Tool: true, LocalOnly: true},
	}
	err := Generate(sites, Options{SitesDir: sitesDir, FastcgiConf: `C:\nginx\conf\fastcgi.conf`, Port: 80})
	if err != nil {
		t.Fatal(err)
	}

	conf, err := os.ReadFile(filepath.Join(sitesDir, "api.test.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"server_name  api.test;",
		`root         "C:/www/api/public";`, // rutas con "/"
		"listen       80;",
		`include       "C:/nginx/conf/fastcgi.conf";`,
		"Laravel detectado",
	} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("api.test.conf no contiene %q:\n%s", want, conf)
		}
	}

	if strings.Contains(string(conf), "allow 127.0.0.1;") {
		t.Error("un sitio normal no debería estar restringido a 127.0.0.1")
	}
	adminer, _ := os.ReadFile(filepath.Join(sitesDir, "adminer.test.conf"))
	if !strings.Contains(string(adminer), "allow 127.0.0.1;") || !strings.Contains(string(adminer), "deny  all;") {
		t.Errorf("adminer.test.conf debería estar restringido a 127.0.0.1:\n%s", adminer)
	}

	if _, err := os.Stat(filepath.Join(sitesDir, "viejo.test.conf")); !os.IsNotExist(err) {
		t.Error("el vhost viejo debería haberse borrado")
	}
	if _, err := os.Stat(filepath.Join(sitesDir, "no-es-conf.txt")); err != nil {
		t.Error("los archivos que no son .conf no se tocan")
	}
}
