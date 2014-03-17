from django.conf.urls import patterns, include, url
from django.conf.urls.static import static
from django.conf import settings

urlpatterns = patterns('',
    url(r'^$', 'blog.views.home', name="home"),
    url(r'^/manage$', 'blog.views.manage_blog', name='manage_blog'),
    url(r'^/add-item$', 'blog.views.add_blog', name="add_blog"),
    url(r'^/delete/(?P<id>\d+)$', 'blog.views.delete_blog', name='delete_blog'),

    url(r'^/login$', 'django.contrib.auth.views.login', {'template_name': 'blog/login.html'}, name="login"),
    url(r'^/logout$', 'django.contrib.auth.views.logout_then_login'),
    url(r'^/register$', 'blog.views.register', name="register"),
    url(r'^/activate/(?P<username>[^&]*)&(?P<code>.*)$', 'blog.views.activate', name="activate"),
    url(r'^/selectblogger/(?P<user_id>[^&]*)', 'blog.views.selectblogger', name="click_blogger"),
    url(r'^/photo/(?P<id>\d+)$', 'blog.views.get_photo', name='photo'),
    url(r'^/get-blog$', 'blog.views.get_blog'),
) + static(settings.STATIC_URL, document_root=settings.STATIC_ROOT)