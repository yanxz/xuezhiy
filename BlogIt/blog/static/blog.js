var req;

// Sends a new request to update the blogger and blog list
function sendRequest() {
	if (window.XMLHttpRequest) {
		req = new XMLHttpRequest();
	} else {
		req = new ActiveXObject("Microsoft.XMLHTTP");
	}
	req.onreadystatechange = handleResponse;
	req.open("GET", "/blog/get-blog", true);
	req.send();
}

function handleResponse() {
	if (req.readyState != 4 || req.status != 200) {
		return;
	}

	// update the blogger list.
	var blogger_list = document.getElementById("blogger-list");
	while (blogger_list.hasChildNodes()) {
		blogger_list.removeChild(blogger_list.firstChild);
	}

	var xmlData = req.responseXML;
	var users = xmlData.getElementsByTagName("user");
	var sel_users = xmlData.getElementsByTagName("sel_blogger");

	var header = document.createElement("li");
	header.className = "active";
	header.innerHTML = "<a href=\"#\">Bloggers</a>";
	blogger_list.appendChild(header);

	for (var i = 0; i < users.length; ++i) {
		var id = users[i].getElementsByTagName("id")[0].textContent;
		var username = users[i].getElementsByTagName("name")[0].textContent;

		var newBlogger = document.createElement("li");

		var isin = false;
		for (var j = 0; j < sel_users.length; ++j) {
			var sel_id = sel_users[j].getElementsByTagName("id")[0].textContent;
			if (sel_id == id) {
				isin = true;
				break;
			}
		}

		if (isin) {
			newBlogger.style.cssText = "background-color: #eeeeee";
		}
		newBlogger.innerHTML = "<a href=\"/blog/selectblogger/" + id + "\">" + username + "</a>";
		blogger_list.appendChild(newBlogger);
	}

	// update the blogs
	var blog_list = document.getElementById("blog-list");
	while (blog_list.hasChildNodes()) {
		blog_list.removeChild(blog_list.firstChild);
	}

	var blogs = xmlData.getElementsByTagName("blog");
	if (blogs.length == 0) {
		var notice = document.createElement("h2");
		notice.className = "blog-post-title";
		notice.style.cssText = "margin-top:40px";
		notice.innerHTML = "No blogs found with chosen bloggers.";
		blog_list.appendChild(notice);
	}

	for (var i = 0; i < blogs.length; ++i) {
		//var id = blogs[i].getElementsByTagName("id")[0].textContent;
		var owner = blogs[i].getElementsByTagName("owner")[0].textContent;
		var post_time = blogs[i].getElementsByTagName("post_time")[0].textContent;
		var picture_id = blogs[i].getElementsByTagName("id");
		var content = blogs[i].getElementsByTagName("content")[0].textContent;

		var newBlog = document.createElement("div");
		if (picture_id.length != 0) {
			picture_id = picture_id[0].textContent;
			newBlog.innerHTML = "<h2 class=\"blog-post-title\">" + owner + 
			 "</h2><div class=\"blog-post-meta\">" + post_time + "<br></div>" +
			 "<img src=\"/blog/photo/" + picture_id + "\">" +
			 "<div class=\"blog-post\">" + content + "</div><hr>";
		} else {
			newBlog.innerHTML = "<h2 class=\"blog-post-title\">" + owner + 
			 "</h2><div class=\"blog-post-meta\">" + post_time + "<br></div>" +
			 "<div class=\"blog-post\">" + content + "</div><hr>";
		}
		blog_list.appendChild(newBlog);
	}
}

// Causes the sendRequest function to run every 5 seconds
window.setInterval(sendRequest, 10000);