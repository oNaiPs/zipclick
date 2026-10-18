package main

import (
	"os"
	"strings"
)

// Every string a person reads, in each language ZipClick speaks. The language
// is the first of the user's Windows display languages that is in the catalog
// (ZIPCLICK_LANG goes first, for testing), else English.
//
// Strings that name Windows' own menus ("Show more options", "Extract All…")
// must match what that Windows says, so check a translation against the real
// menu, not a dictionary.

type messages struct {
	InstallPrompt, Installed, AlmostDone, Removed string

	CouldNotExtract string // %s: the archive's name
	Encrypted       string
	BadPassword     string
	Empty           string
	DiskFull        string
	Damaged         string
	NotArchive      string
	Method          string // %d: the compression method number
	TryExtractAll   string

	PasswordFor   string // %s: the archive's name, quoted
	PasswordAsk   string
	PasswordWrong string
	OK            string

	Extracting string // %s: the archive's name, quoted the way the language quotes
	Cancel     string
	Stopping   string
	Of         string // "%s of %s": done, total
	FewSeconds string
	Seconds    string // %d
	Minute     string
	Minutes    string // %d
	Bytes      string
	Units      [4]string // KB, MB, GB, TB
	Decimal    string

	// Written to the registry at install time.
	VerbExtract, VerbExtractHere, AppDescription string
	TypeName                                     string // %s: "ZIP", "RAR", "7-Zip"...
}

var catalog = map[string]*messages{
	"en":    &en,
	"pt-PT": &ptPT,
	"pt-BR": &ptBR,
	"es":    &es,
	"fr":    &fr,
	"de":    &de,
	"it":    &it,
}

// msg is the language in use; main sets it before anything is shown.
var msg = &en

func setLanguage() {
	prefs := preferredLanguages()
	if v := os.Getenv("ZIPCLICK_LANG"); v != "" {
		prefs = append([]string{v}, prefs...)
	}
	msg = catalog[pickLanguage(prefs)]
}

// pickLanguage maps preferences like "pt-BR", "de-AT" or "fr" to a catalog
// key: an exact match first, then the base language. Portuguese other than
// Brazil's reads best as Portugal's.
func pickLanguage(prefs []string) string {
	for _, p := range prefs {
		p = strings.ReplaceAll(p, "_", "-")
		for k := range catalog {
			if strings.EqualFold(p, k) {
				return k
			}
		}
		base, _, _ := strings.Cut(strings.ToLower(p), "-")
		if base == "pt" {
			return "pt-PT"
		}
		if _, ok := catalog[base]; ok {
			return base
		}
	}
	return "en"
}

var en = messages{
	InstallPrompt: "Make double-clicking a .zip, .rar, .7z or .tar.gz file extract it, like on a Mac?\n\n" +
		"ZipClick installs for your user only. No administrator rights needed.",
	Installed: "ZipClick is installed and already opens .zip files.\n\nThe first time you open a .rar, .7z or other archive, Windows will ask which app to use. Choose ZipClick, then Always.",
	AlmostDone: "Almost done. Windows only lets you choose the default app yourself.\n\n" +
		"In the Settings page that opens next, click the box under .zip and choose ZipClick. The other archive types are on the same page.\n\n" +
		"If you skip them, Windows asks the first time you open a .rar or .7z: choose ZipClick, then Always.",
	Removed: "ZipClick has been removed. Double-clicking a .zip opens it in Explorer again.",

	CouldNotExtract: "Could not extract %s.",
	Encrypted:       "The archive is password-protected.",
	BadPassword:     "The password is not correct.",
	Empty:           "The archive is empty.",
	DiskFull:        "There isn't enough free space on the drive to extract it.",
	Damaged:         "The archive isn't valid, or it is damaged.",
	NotArchive:      "ZipClick doesn't recognise this kind of file.",
	Method:          "It uses a compression method ZipClick doesn't support (%d).",
	TryExtractAll:   "Right-click the file > Extract All… to use Windows' own extractor instead.",

	PasswordFor:   "“%s” is protected with a password.",
	PasswordAsk:   "Enter the password to extract it:",
	PasswordWrong: "That password isn't right. Try again:",
	OK:            "OK",

	Extracting: "Extracting “%s”",
	Cancel:     "Cancel",
	Stopping:   "Stopping…",
	Of:         "%s of %s",
	FewSeconds: "a few seconds left",
	Seconds:    "about %d seconds left",
	Minute:     "about a minute left",
	Minutes:    "about %d minutes left",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ".",

	VerbExtract:     "Extract",
	VerbExtractHere: "Extract here (ZipClick)",
	TypeName:        "%s archive",
	AppDescription:  "Extract .zip, .rar, .7z and .tar.gz files with a double-click, like on a Mac.",
}

var ptPT = messages{
	InstallPrompt: "Fazer com que um duplo clique num ficheiro .zip, .rar, .7z ou .tar.gz o extraia, como num Mac?\n\n" +
		"O ZipClick é instalado só para o seu utilizador. Não precisa de permissões de administrador.",
	Installed: "O ZipClick está instalado e já abre os ficheiros .zip.\n\nNa primeira vez que abrir um .rar, .7z ou outro arquivo, o Windows pergunta que aplicação usar. Escolha ZipClick e depois Sempre.",
	AlmostDone: "Quase pronto. O Windows só deixa que seja o próprio utilizador a escolher a aplicação predefinida.\n\n" +
		"Na página das Definições que vai abrir, clique na caixa por baixo de .zip e escolha ZipClick. Os outros tipos de arquivo estão na mesma página.\n\n" +
		"Se os saltar, o Windows pergunta na primeira vez que abrir um .rar ou .7z: escolha ZipClick e depois Sempre.",
	Removed: "O ZipClick foi removido. Um duplo clique num .zip volta a abri-lo no Explorador de Ficheiros.",

	CouldNotExtract: "Não foi possível extrair %s.",
	Encrypted:       "O arquivo está protegido por palavra-passe.",
	BadPassword:     "A palavra-passe não está correta.",
	Empty:           "O arquivo está vazio.",
	DiskFull:        "Não há espaço livre suficiente no disco para o extrair.",
	Damaged:         "O arquivo não é válido, ou está danificado.",
	NotArchive:      "O ZipClick não reconhece este tipo de ficheiro.",
	Method:          "Usa um método de compressão que o ZipClick não suporta (%d).",
	TryExtractAll:   "Clique com o botão direito no ficheiro > Extrair Tudo… para usar o extrator do próprio Windows.",

	PasswordFor:   "«%s» está protegido por palavra-passe.",
	PasswordAsk:   "Introduza a palavra-passe para o extrair:",
	PasswordWrong: "A palavra-passe não está correta. Tente outra vez:",
	OK:            "OK",

	Extracting: "A extrair «%s»",
	Cancel:     "Cancelar",
	Stopping:   "A parar…",
	Of:         "%s de %s",
	FewSeconds: "faltam poucos segundos",
	Seconds:    "faltam cerca de %d segundos",
	Minute:     "falta cerca de um minuto",
	Minutes:    "faltam cerca de %d minutos",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Extrair",
	VerbExtractHere: "Extrair aqui (ZipClick)",
	TypeName:        "Arquivo %s",
	AppDescription:  "Extraia ficheiros .zip, .rar, .7z e .tar.gz com um duplo clique, como num Mac.",
}

var ptBR = messages{
	InstallPrompt: "Fazer com que um clique duplo em um arquivo .zip, .rar, .7z ou .tar.gz o extraia, como no Mac?\n\n" +
		"O ZipClick é instalado só para o seu usuário. Não precisa de permissão de administrador.",
	Installed: "O ZipClick está instalado e já abre os arquivos .zip.\n\nNa primeira vez que você abrir um .rar, .7z ou outro arquivo compactado, o Windows vai perguntar qual aplicativo usar. Escolha ZipClick e depois Sempre.",
	AlmostDone: "Quase pronto. O Windows só deixa você mesmo escolher o aplicativo padrão.\n\n" +
		"Na página de Configurações que vai abrir, clique na caixa abaixo de .zip e escolha ZipClick. Os outros tipos de arquivo compactado estão na mesma página.\n\n" +
		"Se você pular essa parte, o Windows pergunta na primeira vez que você abrir um .rar ou .7z: escolha ZipClick e depois Sempre.",
	Removed: "O ZipClick foi removido. Um clique duplo em um .zip volta a abri-lo no Explorador de Arquivos.",

	CouldNotExtract: "Não foi possível extrair %s.",
	Encrypted:       "O arquivo está protegido por senha.",
	BadPassword:     "A senha não está correta.",
	Empty:           "O arquivo está vazio.",
	DiskFull:        "Não há espaço livre suficiente no disco para extraí-lo.",
	Damaged:         "O arquivo não é válido, ou está corrompido.",
	NotArchive:      "O ZipClick não reconhece este tipo de arquivo.",
	Method:          "Ele usa um método de compactação que o ZipClick não suporta (%d).",
	TryExtractAll:   "Clique com o botão direito no arquivo > Extrair Tudo… para usar o extrator do próprio Windows.",

	PasswordFor:   "“%s” está protegido por senha.",
	PasswordAsk:   "Digite a senha para extraí-lo:",
	PasswordWrong: "A senha não está correta. Tente de novo:",
	OK:            "OK",

	Extracting: "Extraindo “%s”",
	Cancel:     "Cancelar",
	Stopping:   "Parando…",
	Of:         "%s de %s",
	FewSeconds: "faltam alguns segundos",
	Seconds:    "faltam cerca de %d segundos",
	Minute:     "falta cerca de um minuto",
	Minutes:    "faltam cerca de %d minutos",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Extrair",
	VerbExtractHere: "Extrair aqui (ZipClick)",
	TypeName:        "Arquivo %s",
	AppDescription:  "Extraia arquivos .zip, .rar, .7z e .tar.gz com um clique duplo, como no Mac.",
}

var es = messages{
	InstallPrompt: "¿Quieres que al hacer doble clic en un archivo .zip, .rar, .7z o .tar.gz se extraiga, como en un Mac?\n\n" +
		"ZipClick se instala solo para tu usuario. No hacen falta permisos de administrador.",
	Installed: "ZipClick está instalado y ya abre los archivos .zip.\n\nLa primera vez que abras un .rar, .7z u otro archivo comprimido, Windows te preguntará qué aplicación usar. Elige ZipClick y luego Siempre.",
	AlmostDone: "Casi listo. Windows solo te deja elegir a ti la aplicación predeterminada.\n\n" +
		"En la página de Configuración que se abrirá, haz clic en el cuadro bajo .zip y elige ZipClick. Los demás tipos de archivo comprimido están en la misma página.\n\n" +
		"Si te los saltas, Windows te preguntará la primera vez que abras un .rar o .7z: elige ZipClick y luego Siempre.",
	Removed: "ZipClick se ha eliminado. Al hacer doble clic en un .zip se vuelve a abrir en el Explorador de archivos.",

	CouldNotExtract: "No se ha podido extraer %s.",
	Encrypted:       "El archivo está protegido con contraseña.",
	BadPassword:     "La contraseña no es correcta.",
	Empty:           "El archivo está vacío.",
	DiskFull:        "No hay suficiente espacio libre en el disco para extraerlo.",
	Damaged:         "El archivo no es válido, o está dañado.",
	NotArchive:      "ZipClick no reconoce este tipo de archivo.",
	Method:          "Usa un método de compresión que ZipClick no admite (%d).",
	TryExtractAll:   "Haz clic con el botón derecho en el archivo > Extraer todo… para usar el extractor del propio Windows.",

	PasswordFor:   "«%s» está protegido con contraseña.",
	PasswordAsk:   "Escribe la contraseña para extraerlo:",
	PasswordWrong: "La contraseña no es correcta. Inténtalo de nuevo:",
	OK:            "Aceptar",

	Extracting: "Extrayendo «%s»",
	Cancel:     "Cancelar",
	Stopping:   "Deteniendo…",
	Of:         "%s de %s",
	FewSeconds: "quedan unos segundos",
	Seconds:    "quedan unos %d segundos",
	Minute:     "queda alrededor de un minuto",
	Minutes:    "quedan unos %d minutos",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Extraer",
	VerbExtractHere: "Extraer aquí (ZipClick)",
	TypeName:        "Archivo %s",
	AppDescription:  "Extrae archivos .zip, .rar, .7z y .tar.gz con un doble clic, como en un Mac.",
}

// French puts a no-break space inside « » and before ? and :.
var fr = messages{
	InstallPrompt: "Extraire les fichiers .zip, .rar, .7z ou .tar.gz d’un double-clic, comme sur un Mac ?\n\n" +
		"ZipClick s’installe uniquement pour votre compte, sans droits d’administrateur.",
	Installed: "ZipClick est installé et ouvre déjà les fichiers .zip.\n\nLa première fois que vous ouvrirez un .rar, un .7z ou une autre archive, Windows vous demandera quelle application utiliser. Choisissez ZipClick, puis Toujours.",
	AlmostDone: "Presque terminé. Windows ne laisse que vous choisir l’application par défaut.\n\n" +
		"Dans la page Paramètres qui va s’ouvrir, cliquez sur la case sous .zip et choisissez ZipClick. Les autres types d’archive sont sur la même page.\n\n" +
		"Si vous les laissez de côté, Windows vous le demandera la première fois que vous ouvrirez un .rar ou un .7z : choisissez ZipClick, puis Toujours.",
	Removed: "ZipClick a été supprimé. Un double-clic sur un .zip l’ouvre à nouveau dans l’Explorateur de fichiers.",

	CouldNotExtract: "Impossible d’extraire %s.",
	Encrypted:       "L’archive est protégée par un mot de passe.",
	BadPassword:     "Le mot de passe est incorrect.",
	Empty:           "L’archive est vide.",
	DiskFull:        "Il n’y a pas assez d’espace libre sur le disque pour l’extraire.",
	Damaged:         "L’archive n’est pas valide, ou elle est endommagée.",
	NotArchive:      "ZipClick ne reconnaît pas ce type de fichier.",
	Method:          "Elle utilise une méthode de compression que ZipClick ne prend pas en charge (%d).",
	TryExtractAll:   "Faites un clic droit sur le fichier > Extraire tout… pour utiliser l’extracteur de Windows.",

	PasswordFor:   "« %s » est protégée par un mot de passe.",
	PasswordAsk:   "Saisissez le mot de passe pour l’extraire :",
	PasswordWrong: "Mot de passe incorrect. Réessayez :",
	OK:            "OK",

	Extracting: "Extraction de « %s »",
	Cancel:     "Annuler",
	Stopping:   "Arrêt…",
	Of:         "%s sur %s",
	FewSeconds: "encore quelques secondes",
	Seconds:    "encore environ %d secondes",
	Minute:     "encore environ une minute",
	Minutes:    "encore environ %d minutes",
	Bytes:      "octets",
	Units:      [4]string{"Ko", "Mo", "Go", "To"},
	Decimal:    ",",

	VerbExtract:     "Extraire",
	VerbExtractHere: "Extraire ici (ZipClick)",
	TypeName:        "Archive %s",
	AppDescription:  "Extrayez les fichiers .zip, .rar, .7z et .tar.gz d’un double-clic, comme sur un Mac.",
}

var de = messages{
	InstallPrompt: "Sollen .zip-, .rar-, .7z- und .tar.gz-Dateien per Doppelklick entpackt werden, wie auf einem Mac?\n\n" +
		"ZipClick wird nur für Ihr Benutzerkonto installiert. Administratorrechte sind nicht nötig.",
	Installed: "ZipClick ist installiert und öffnet bereits .zip-Dateien.\n\nWenn Sie zum ersten Mal eine .rar-, .7z- oder andere Archivdatei öffnen, fragt Windows, welche App verwendet werden soll. Wählen Sie ZipClick und dann Immer.",
	AlmostDone: "Fast fertig. Die Standard-App können unter Windows nur Sie selbst festlegen.\n\n" +
		"Klicken Sie auf der Einstellungsseite, die sich gleich öffnet, auf das Feld unter .zip und wählen Sie ZipClick. Die anderen Archivtypen finden Sie auf derselben Seite.\n\n" +
		"Wenn Sie sie überspringen, fragt Windows beim ersten Öffnen einer .rar- oder .7z-Datei: Wählen Sie ZipClick und dann Immer.",
	Removed: "ZipClick wurde entfernt. Ein Doppelklick auf eine .zip-Datei öffnet sie wieder im Datei-Explorer.",

	CouldNotExtract: "%s konnte nicht entpackt werden.",
	Encrypted:       "Das Archiv ist kennwortgeschützt.",
	BadPassword:     "Das Kennwort ist falsch.",
	Empty:           "Das Archiv ist leer.",
	DiskFull:        "Auf dem Laufwerk ist nicht genug Speicherplatz frei, um es zu entpacken.",
	Damaged:         "Das Archiv ist ungültig oder beschädigt.",
	NotArchive:      "ZipClick kennt diesen Dateityp nicht.",
	Method:          "Sie verwendet eine Komprimierungsmethode, die ZipClick nicht unterstützt (%d).",
	TryExtractAll:   "Klicken Sie mit der rechten Maustaste auf die Datei > Alle extrahieren…, um das Entpacken von Windows zu verwenden.",

	PasswordFor:   "„%s“ ist kennwortgeschützt.",
	PasswordAsk:   "Geben Sie das Kennwort zum Entpacken ein:",
	PasswordWrong: "Das Kennwort ist falsch. Versuchen Sie es noch einmal:",
	OK:            "OK",

	Extracting: "„%s“ wird entpackt",
	Cancel:     "Abbrechen",
	Stopping:   "Wird angehalten…",
	Of:         "%s von %s",
	FewSeconds: "noch wenige Sekunden",
	Seconds:    "noch etwa %d Sekunden",
	Minute:     "noch etwa eine Minute",
	Minutes:    "noch etwa %d Minuten",
	Bytes:      "Byte",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Entpacken",
	VerbExtractHere: "Hier entpacken (ZipClick)",
	TypeName:        "%s-Archiv",
	AppDescription:  ".zip-, .rar-, .7z- und .tar.gz-Dateien per Doppelklick entpacken, wie auf einem Mac.",
}

var it = messages{
	InstallPrompt: "Vuoi che un doppio clic su un file .zip, .rar, .7z o .tar.gz lo estragga, come su un Mac?\n\n" +
		"ZipClick si installa solo per il tuo utente: non servono diritti di amministratore.",
	Installed: "ZipClick è installato e apre già i file .zip.\n\nLa prima volta che apri un .rar, un .7z o un altro archivio, Windows ti chiederà quale app usare. Scegli ZipClick e poi Sempre.",
	AlmostDone: "Quasi fatto. Windows permette solo a te di scegliere l’app predefinita.\n\n" +
		"Nella pagina Impostazioni che si aprirà, fai clic sulla casella sotto .zip e scegli ZipClick. Gli altri tipi di archivio sono nella stessa pagina.\n\n" +
		"Se li salti, Windows te lo chiederà la prima volta che apri un .rar o un .7z: scegli ZipClick e poi Sempre.",
	Removed: "ZipClick è stato rimosso. Un doppio clic su un .zip lo apre di nuovo in Esplora file.",

	CouldNotExtract: "Impossibile estrarre %s.",
	Encrypted:       "L’archivio è protetto da password.",
	BadPassword:     "La password non è corretta.",
	Empty:           "L’archivio è vuoto.",
	DiskFull:        "Non c’è abbastanza spazio libero sul disco per estrarlo.",
	Damaged:         "L’archivio non è valido, oppure è danneggiato.",
	NotArchive:      "ZipClick non riconosce questo tipo di file.",
	Method:          "Usa un metodo di compressione che ZipClick non supporta (%d).",
	TryExtractAll:   "Fai clic con il tasto destro sul file > Estrai tutto… per usare l’estrattore di Windows.",

	PasswordFor:   "«%s» è protetto da password.",
	PasswordAsk:   "Inserisci la password per estrarlo:",
	PasswordWrong: "La password non è corretta. Riprova:",
	OK:            "OK",

	Extracting: "Estrazione di «%s»",
	Cancel:     "Annulla",
	Stopping:   "Interruzione…",
	Of:         "%s di %s",
	FewSeconds: "mancano pochi secondi",
	Seconds:    "mancano circa %d secondi",
	Minute:     "manca circa un minuto",
	Minutes:    "mancano circa %d minuti",
	Bytes:      "byte",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Estrai",
	VerbExtractHere: "Estrai qui (ZipClick)",
	TypeName:        "Archivio %s",
	AppDescription:  "Estrai file .zip, .rar, .7z e .tar.gz con un doppio clic, come su un Mac.",
}
