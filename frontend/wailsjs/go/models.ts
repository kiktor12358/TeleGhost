export namespace appcore {
	
	export class ReplyPreview {
	    author_name: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new ReplyPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.author_name = source["author_name"];
	        this.content = source["content"];
	    }
	}

}

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
	    LastMessageTime: number;
	    LastSeen: string;
	    IsOnline: boolean;
	    ChatID: string;
	    UnreadCount: number;
	
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
	        this.LastMessageTime = source["LastMessageTime"];
	        this.LastSeen = source["LastSeen"];
	        this.IsOnline = source["IsOnline"];
	        this.ChatID = source["ChatID"];
	        this.UnreadCount = source["UnreadCount"];
	    }
	}
	export class FolderInfo {
	    ID: string;
	    Name: string;
	    Icon: string;
	    ChatIDs: string[];
	    Position: number;
	    UnreadCount: number;
	
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
	        this.UnreadCount = source["UnreadCount"];
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
	    ReplyToID: string;
	    ReplyPreview?: appcore.ReplyPreview;
	
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
	        this.ReplyToID = source["ReplyToID"];
	        this.ReplyPreview = this.convertValues(source["ReplyPreview"], appcore.ReplyPreview);
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

}

