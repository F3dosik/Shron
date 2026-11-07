export namespace backend {
	
	export class ChosenFile {
	    path: string;
	    name: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new ChosenFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	    }
	}
	export class Generated {
	    password: string;
	    entropyBits: number;
	    strength: string;
	
	    static createFrom(source: any = {}) {
	        return new Generated(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.password = source["password"];
	        this.entropyBits = source["entropyBits"];
	        this.strength = source["strength"];
	    }
	}
	export class GeneratorOptions {
	    length: number;
	    lower: boolean;
	    upper: boolean;
	    digits: boolean;
	    symbols: boolean;
	    noAmbiguous: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GeneratorOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.length = source["length"];
	        this.lower = source["lower"];
	        this.upper = source["upper"];
	        this.digits = source["digits"];
	        this.symbols = source["symbols"];
	        this.noAmbiguous = source["noAmbiguous"];
	    }
	}
	export class ImportFailure {
	    name: string;
	    type: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportFailure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.error = source["error"];
	    }
	}
	export class ImportResult {
	    sourceLogin: string;
	    exportedAt: string;
	    total: number;
	    created: number;
	    updated: number;
	    skipped: number;
	    failed: ImportFailure[];
	    interrupted: boolean;
	    interruptReason: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceLogin = source["sourceLogin"];
	        this.exportedAt = source["exportedAt"];
	        this.total = source["total"];
	        this.created = source["created"];
	        this.updated = source["updated"];
	        this.skipped = source["skipped"];
	        this.failed = this.convertValues(source["failed"], ImportFailure);
	        this.interrupted = source["interrupted"];
	        this.interruptReason = source["interruptReason"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Secret {
	    name: string;
	    type: string;
	    metadata: string;
	    group: string;
	    createdAt: string;
	    updatedAt: string;
	    login: string;
	    password: string;
	    uris: string;
	    text: string;
	    cardNumber: string;
	    cardHolder: string;
	    cardExpiry: string;
	    cardCvv: string;
	    fileSize: number;
	
	    static createFrom(source: any = {}) {
	        return new Secret(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.metadata = source["metadata"];
	        this.group = source["group"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.login = source["login"];
	        this.password = source["password"];
	        this.uris = source["uris"];
	        this.text = source["text"];
	        this.cardNumber = source["cardNumber"];
	        this.cardHolder = source["cardHolder"];
	        this.cardExpiry = source["cardExpiry"];
	        this.cardCvv = source["cardCvv"];
	        this.fileSize = source["fileSize"];
	    }
	}
	export class SecretInput {
	    name: string;
	    type: string;
	    metadata: string;
	    group: string;
	    login: string;
	    password: string;
	    uris: string;
	    text: string;
	    cardNumber: string;
	    cardHolder: string;
	    cardExpiry: string;
	    cardCvv: string;
	    filePath: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.metadata = source["metadata"];
	        this.group = source["group"];
	        this.login = source["login"];
	        this.password = source["password"];
	        this.uris = source["uris"];
	        this.text = source["text"];
	        this.cardNumber = source["cardNumber"];
	        this.cardHolder = source["cardHolder"];
	        this.cardExpiry = source["cardExpiry"];
	        this.cardCvv = source["cardCvv"];
	        this.filePath = source["filePath"];
	    }
	}
	export class SecretSummary {
	    name: string;
	    type: string;
	    metadata: string;
	    group: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.metadata = source["metadata"];
	        this.group = source["group"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class State {
	    configured: boolean;
	    serverAddress: string;
	    hasCaCert: boolean;
	    autoLockMinutes: number;
	    login: string;
	    unlocked: boolean;
	    adminMode: boolean;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.serverAddress = source["serverAddress"];
	        this.hasCaCert = source["hasCaCert"];
	        this.autoLockMinutes = source["autoLockMinutes"];
	        this.login = source["login"];
	        this.unlocked = source["unlocked"];
	        this.adminMode = source["adminMode"];
	    }
	}
	export class TemporaryUser {
	    login: string;
	    password: string;
	    expiresAt: string;
	
	    static createFrom(source: any = {}) {
	        return new TemporaryUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.password = source["password"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	export class UserRow {
	    login: string;
	    createdAt: string;
	    secretCount: number;
	    temporaryUntil: string;
	    temporaryExpired: boolean;
	    kdfTime: number;
	    kdfMemoryMiB: number;
	
	    static createFrom(source: any = {}) {
	        return new UserRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login = source["login"];
	        this.createdAt = source["createdAt"];
	        this.secretCount = source["secretCount"];
	        this.temporaryUntil = source["temporaryUntil"];
	        this.temporaryExpired = source["temporaryExpired"];
	        this.kdfTime = source["kdfTime"];
	        this.kdfMemoryMiB = source["kdfMemoryMiB"];
	    }
	}

}

