export namespace main {
	
	export class AppAboutInfo {
	    app_version: string;
	    i2p_version: string;
	    i2p_path: string;
	    author: string;
	    license: string;
	
	    static createFrom(source: any = {}) {
	        return new AppAboutInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app_version = source["app_version"];
	        this.i2p_version = source["i2p_version"];
	        this.i2p_path = source["i2p_path"];
	        this.author = source["author"];
	        this.license = source["license"];
	    }
	}
	export class ContactInfo {
	    ID: string;
	    Nickname: string;
	    PublicKey: string;
	    Avatar: string;
	    I2PAddress: string;
	    LastMessage: string;
	    LastSeen: string;
	    IsOnline: boolean;
	    ChatID: string;
	
	    static createFrom(source: any = {}) {
	        return new ContactInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Nickname = source["Nickname"];
	        this.PublicKey = source["PublicKey"];
	        this.Avatar = source["Avatar"];
	        this.I2PAddress = source["I2PAddress"];
	        this.LastMessage = source["LastMessage"];
	        this.LastSeen = source["LastSeen"];
	        this.IsOnline = source["IsOnline"];
	        this.ChatID = source["ChatID"];
	    }
	}
	export class FolderInfo {
	    ID: string;
	    Name: string;
	    Icon: string;
	    ChatIDs: string[];
	    Position: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Icon = source["Icon"];
	        this.ChatIDs = source["ChatIDs"];
	        this.Position = source["Position"];
	    }
	}
	export class MessageInfo {
	    ID: string;
	    Content: string;
	    Timestamp: number;
	    IsOutgoing: boolean;
	    Status: string;
	    ContentType: string;
	    FileCount: number;
	    TotalSize: number;
	    Filenames: string[];
	    Attachments: any[];
	
	    static createFrom(source: any = {}) {
	        return new MessageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Content = source["Content"];
	        this.Timestamp = source["Timestamp"];
	        this.IsOutgoing = source["IsOutgoing"];
	        this.Status = source["Status"];
	        this.ContentType = source["ContentType"];
	        this.FileCount = source["FileCount"];
	        this.TotalSize = source["TotalSize"];
	        this.Filenames = source["Filenames"];
	        this.Attachments = source["Attachments"];
	    }
	}
	export class RouterSettings {
	    TunnelLength: number;
	    LogToFile: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RouterSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TunnelLength = source["TunnelLength"];
	        this.LogToFile = source["LogToFile"];
	    }
	}
	export class UserInfo {
	    ID: string;
	    Nickname: string;
	    Avatar: string;
	    PublicKey: string;
	    Destination: string;
	    Fingerprint: string;
	    Mnemonic: string;
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Nickname = source["Nickname"];
	        this.Avatar = source["Avatar"];
	        this.PublicKey = source["PublicKey"];
	        this.Destination = source["Destination"];
	        this.Fingerprint = source["Fingerprint"];
	        this.Mnemonic = source["Mnemonic"];
	    }
	}

}

export namespace profiles {
	
	export class ProfileMetadata {
	    id: string;
	    display_name: string;
	    user_id: string;
	    avatar_path: string;
	    use_pin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileMetadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.display_name = source["display_name"];
	        this.user_id = source["user_id"];
	        this.avatar_path = source["avatar_path"];
	        this.use_pin = source["use_pin"];
	    }
	}

}

