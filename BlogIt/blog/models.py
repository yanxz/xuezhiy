from django.db import models
from django.contrib.auth.models import User

# User class for built-in authentication module

# Blog class
class Blog(models.Model):
    owner = models.ForeignKey(User)
    content = models.CharField(max_length=160)
    date_posted = models.DateTimeField()
    picture = models.ImageField(upload_to='blogs', blank=True)

    def __unicode__(self):
        return self.content

    @staticmethod
    def get_blogs(owner):
        return Blog.objects.filter(owner=owner).order_by('date_posted')

class AuthCode(models.Model):
    code = models.CharField(max_length=20)
    owner = models.ForeignKey(User)

    def __unicode__(self):
        return self.owner.username + " " + self.code

# Follow class, each instance of this class represents a follow relationship.
class Follow(models.Model):
    # follower and the id of the user the follower is following
    follower = models.ForeignKey(User, null=True, default=None)
    followeeId = models.IntegerField()

    def __unicode__(self):
        return follower.username + "->" + followeeId

    # given a user, get his/her following users, when the user is anonymous
    # get those follows with anonymous followers.
    @staticmethod
    def get_followings(user):
        if user.is_authenticated():
            follows = Follow.objects.filter(follower=user)
        else:
            follows = Follow.objects.filter(follower__isnull=True)
        
        bloggers = []
        for f in follows:
            user = (User.objects.filter(id=f.followeeId))[0]
            bloggers.append(user)
        return bloggers