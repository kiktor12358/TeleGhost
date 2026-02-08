export namespace main {
	
	export class ContactInfo {
	    id: string;
	    nickname: string;
	    publicKey: string;
	    avatar: string;
	    i2pAddress: string;
	    lastMessage: string;
	    lastSeen: string;
	    isOnline: boolean;
	    chatId: string;
	
	    static createFrom(source: any = {}) {
	        return new ContactInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.nickname = source["nickname"];
	        this.publicKey = source["publicKey"];
	        this.avatar = source["avatar"];
	        this.i2pAddress = source["i2pAddress"];
	        this.lastMessage = source["lastMessage"];
	        this.lastSeen = source["lastSeen"];
	        this.isOnline = source["isOnline"];
	        this.chatId = source["chatId"];
	    }
	}
	export class FolderInfo {
	    id: string;
	    name: string;
	    icon: string;
	    chatIds: string[];
	    position: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.chatIds = source["chatIds"];
	        this.position = source["position"];
	    }
	}
	export class MessageInfo {
	    id: string;
	    content: string;
	    timestamp: number;
	    isOutgoing: boolean;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	        this.isOutgoing = source["isOutgoing"];
	        this.status = source["status"];
	    }
	}
	export class UserInfo {
	    id: string;
	    nickname: string;
	    avatar: string;
	    publicKey: string;
	    destination: string;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.publicKey = source["publicKey"];
	        this.destination = source["destination"];
	        this.fingerprint = source["fingerprint"];
	    }
	}

}

