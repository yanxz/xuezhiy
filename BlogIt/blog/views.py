from django.shortcuts import render
from django.core.urlresolvers import reverse
from django.db.models import Q

from django.contrib.auth.decorators import login_required
from django.shortcuts import redirect

from django.contrib.auth.models import User
from django.contrib.auth import login, authenticate
from django.contrib.auth.forms import AuthenticationForm

from blog.models import *
from blog.forms import *

from django.core.mail import send_mail
from django.http import HttpResponse, Http404

from datetime import datetime, date, time
from mimetypes import guess_type

import string
import random

def home(request):
    users = User.objects.all()
    # the users that the current user is following.
    selected_bloggers = Follow.get_followings(request.user)
    blogs = Blog.objects.filter(owner__in=selected_bloggers).order_by('-date_posted')

    # logged_in user.
    if request.user.is_authenticated():
        context = { 'owner':request.user, 
                    'blogs':blogs, 
                    'users': users, 
                    'bloggers': selected_bloggers}
        return render(request, "blog/home.html", context)
    # anonymous user.
    else:
        context = {'blogs':blogs, 'users': users, 'bloggers': selected_bloggers}
        return render(request, "blog/anonymous.html", context)

# called when browser clicked the blogger names in sidebar.
def selectblogger(request, user_id):
    # logged_in user
    if request.user.is_authenticated():
        # select one blogger
        if not Follow.objects.filter(follower=request.user, followeeId=user_id):
            new_follow = Follow(follower=request.user, followeeId=user_id)
            new_follow.save()
        # unselect one blogger
        else:
            Follow.objects.get(follower=request.user, followeeId=user_id).delete()
        return redirect(reverse('home'))
    # anonymous user.
    else:
        Follow.objects.filter(follower__isnull=True).delete()
        anonymous_follow = Follow(followeeId=user_id)
        anonymous_follow.save()
        return redirect(reverse('home'))

# called to deliver static images.
def get_photo(request, id):
    blog = Blog.objects.filter(id=id)
    if len(blog) == 0:
        raise Http404

    content_type = guess_type(blog[0].picture.name)
    return HttpResponse(blog[0].picture, mimetype=content_type)

# manage blog pages
@login_required
def manage_blog(request):
    users = User.objects.all()
    selected_bloggers = Follow.get_followings(request.user)
    blogs = Blog.objects.filter(owner=request.user).order_by('-date_posted')
    context = {'form':BlogForm(), 
                'blogs':blogs, 
                'owner':request.user, 
                'users':users, 
                'bloggers':selected_bloggers}
    return render(request, 'blog/blogging.html', context)

# method called to add blog.
@login_required
def add_blog(request):
    
    new_Blog = Blog(owner = request.user, date_posted = datetime.now())
    form = BlogForm(request.POST, request.FILES, instance=new_Blog)

    if not form.is_valid():
        return redirect(reverse('manage_blog'))

    form.save()
    return redirect(reverse('home'))

# method called to delete blog.
@login_required
def delete_blog(request, id):
    blog = Blog.objects.filter(id=id)
    blog.delete()
    return redirect(reverse('manage_blog'))

# activate registered user
def activate(request, username, code):
    auth = AuthCode.objects.filter(code=code, owner__username=username)
    # this user has not registered or the code is wrong
    if len(auth) <= 0:
        return render(request, "blog/register.html", {})

    user = User.objects.filter(username=username)[0]
    user.is_active = True
    user.save()
    return render(request, "blog/success.html", {'user':user.username})

# register page.
def register(request):
    context = {}

    if request.method == 'GET':
        context['form'] = RegistrationForm()
        return render(request, 'blog/register.html', context)

    form = RegistrationForm(request.POST)
    context['form'] = form
    if not form.is_valid():
        return render(request, 'blog/register.html', context)

    new_user = User.objects.create_user(username = request.POST['username'],
                                        password = request.POST['password1'],
                                        first_name = request.POST['first_name'],
                                        last_name = request.POST['last_name'])
    new_user.is_active = False
    new_user.save()
    code = ''.join(random.choice(string.ascii_uppercase + string.digits) for x in range(20))
    new_authcode = AuthCode(code=code, owner=new_user)
    new_authcode.save()
    # print("first line" + new_user.username)

    # send the email
    email_body = """
Welcome to BlogIt, Please click the link below to verify your
email address and complete the registrantion of your account:

    http://%s%s
""" % (request.get_host(), 
        reverse('activate', args=(new_user.username, code)))

    send_mail('Verify your email address', 
                email_body, 
                'xuezhiy@andrew.cmu.edu', 
                [new_user.username])
    # print url
    # print "================================="
    # print ("http://%s%s:" % (request.get_host(),
    #    reverse('activate', args=(new_user.username, code))))

    context = {'username': new_user.username, 'authcode': new_authcode}
    # successful registration, render activation page
    return render(request, 'blog/activation.html', context)

# for AJAX communication
def get_blog(request):
    users = User.objects.all()
    selected_bloggers = Follow.get_followings(request.user)
    blogs = Blog.objects.filter(owner__in=selected_bloggers).order_by('-date_posted')
    context = {'blogs': blogs, 'users': users, 'bloggers': selected_bloggers}
    return render(request, 
                "blog/bloggers_and_blogs.xml", 
                context, 
                content_type='application/xml')