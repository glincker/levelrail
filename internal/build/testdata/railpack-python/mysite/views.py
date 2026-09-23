from django.http import HttpResponse


def home(request):
    return HttpResponse("levelrail railpack python django fixture")
