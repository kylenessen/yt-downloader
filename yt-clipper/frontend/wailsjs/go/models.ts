export namespace main {
	
	export class ExportOptions {
	    startTime: number;
	    endTime: number;
	    removeAudio: boolean;
	    filename: string;
	    outputDir: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startTime = source["startTime"];
	        this.endTime = source["endTime"];
	        this.removeAudio = source["removeAudio"];
	        this.filename = source["filename"];
	        this.outputDir = source["outputDir"];
	    }
	}
	export class VideoInfo {
	    id: string;
	    title: string;
	    author: string;
	    duration: number;
	    thumbnail: string;
	    videoUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.author = source["author"];
	        this.duration = source["duration"];
	        this.thumbnail = source["thumbnail"];
	        this.videoUrl = source["videoUrl"];
	    }
	}

}

export namespace video {
	
	export class Server {
	
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace youtube {
	
	export class VideoInfo {
	    id: string;
	    title: string;
	    author: string;
	    duration: number;
	    thumbnail: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.author = source["author"];
	        this.duration = source["duration"];
	        this.thumbnail = source["thumbnail"];
	        this.description = source["description"];
	    }
	}

}

