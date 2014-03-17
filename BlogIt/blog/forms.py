from django import forms

from django.contrib.auth.models import User
from models import *

class RegistrationForm(forms.Form):
    username = forms.EmailField(max_length = 45,
                                label = 'Email',
                                widget = forms.EmailInput(
                                    attrs={'class': 'form-control',
                                           'style': 'width:335px',
                                           'placeholder': 'Email'}))
    first_name = forms.CharField(max_length = 20,
                                 widget = forms.TextInput(
                                    attrs={'class': 'form-control',
                                           'style': 'width:335px',
                                           'placeholder': 'first_name'}))
    last_name = forms.CharField(max_length = 20,
                                widget = forms.TextInput(
                                    attrs={'class': 'form-control',
                                           'style': 'width:335px',
                                           'placeholder': 'last_name'}))
    password1 = forms.CharField(max_length = 200,
                                label = 'Password',
                                widget = forms.PasswordInput(
                                    attrs={'class': 'form-control',
                                           'style': 'width:335px',
                                           'placeholder': 'password1'}))
    password2 = forms.CharField(max_length = 200,
                                label = 'Confirm Password',
                                widget = forms.PasswordInput(
                                    attrs={'class': 'form-control',
                                           'style': 'width:335px',
                                           'placeholder': 'password2'}))


    def clean(self):
        cleaned_data = super(RegistrationForm, self).clean()

        password1 = cleaned_data.get('password1')
        password2 = cleaned_data.get('password2')
        if password1 and password2 and password1 != password2:
            raise forms.ValidationError("Passwords did not match.")

        return cleaned_data

    def clean_username(self):
        username = self.cleaned_data.get('username')
        if User.objects.filter(username__exact = username):
            raise forms.ValidationError("Email address is already taken.")

class BlogForm(forms.ModelForm):
    class Meta:
        model = Blog
        exclude = ('owner', 'date_posted')
        widgets = {
          'picture': forms.FileInput(attrs={'class': 'btn'}),
          'content': forms.Textarea(attrs={'class': 'span6 text', 'cols': 60})
        }