curx = _X;
cury = _Y;
_X = Number(curx) + Number(speedx);
_Y = Number(cury) + Number(speedy);
ap = 75 * life / 70;
_alpha = ap;
_xscale = ap * 0.5;
_yscale = ap * 0.5;
speedx = Number(speedx) - Number((random(51) - 25) / ap2);
speedy = Number(speedy) - Number((random(51) - 25) / ap2);
life -= 1;
ap2 += 100;
if(Number(life) < 0)
{
   removeMovieClip("");
   stop();
}
