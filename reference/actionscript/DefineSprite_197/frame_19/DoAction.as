time -= 1;
if(0 >= time)
{
   time = 0;
   if(eval("/:busy") == 1)
   {
      gotoAndPlay(2);
   }
   else
   {
      _root.gotoAndPlay("end");
   }
}
else if(10 >= time)
{
   gotoAndPlay(20);
}
else
{
   gotoAndPlay(2);
}
